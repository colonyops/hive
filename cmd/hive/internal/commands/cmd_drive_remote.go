package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/driveremote"
	"github.com/colonyops/hive/pkg/iojson"
)

func (cmd *DriveCmd) serverFlag() cli.Flag {
	return &cli.StringFlag{
		Name:        "server",
		Usage:       "drive server URL or host (default: HIVE_DRIVE_SERVER, else the one server you are signed in to)",
		Destination: &cmd.server,
	}
}

func (cmd *DriveCmd) jsonFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:        "json",
		Usage:       "output one JSON object per path",
		Destination: &cmd.json,
	}
}

func (cmd *DriveCmd) remote() (*driveremote.Service, error) {
	remote := cmd.app.DriveRemote()
	if remote == nil {
		return nil, errors.New("drive server access is not available in this build")
	}
	return remote, nil
}

func (cmd *DriveCmd) loginCmd() *cli.Command {
	return &cli.Command{
		Name:      "login",
		Usage:     "Sign in to a drive server with an API token",
		ArgsUsage: "<server>",
		Description: `Checks the token with the server, then stores it in the OS keychain under
the server's host. Mint a token in the server's web UI, or with
'hive-drive-server accounts token create'.

The token is read from a prompt, or from one line of stdin with --token-stdin.

Without a keychain (headless Linux, CI), skip login and set HIVE_DRIVE_SERVER
and HIVE_DRIVE_TOKEN instead.

Example: echo "$TOKEN" | hive drive login drive.hivedesktop.com --token-stdin`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "token-stdin",
				Usage:       "read the token from one line of stdin",
				Destination: &cmd.tokenStdin,
			},
		},
		Action: cmd.runLogin,
	}
}

func (cmd *DriveCmd) runLogin(ctx context.Context, c *cli.Command) error {
	remote, err := cmd.remote()
	if err != nil {
		return err
	}
	server := c.Args().First()
	if server == "" {
		return errors.New("login: server is required")
	}
	token, err := cmd.readToken()
	if err != nil {
		return err
	}

	me, err := remote.Login(ctx, server, token)
	if errors.Is(err, driveremote.ErrStoreCredential) {
		fmt.Fprintln(os.Stderr, "The token is valid but could not be stored: no OS keychain is available.")
		fmt.Fprintln(os.Stderr, "Set HIVE_DRIVE_SERVER and HIVE_DRIVE_TOKEN in the environment instead.")
		return err
	}
	if err != nil {
		return err
	}

	fmt.Printf("Signed in as %s\n", me.Email)
	fmt.Printf("Policy: %s\n", me.Policy)
	if me.Quota.Limit == nil {
		fmt.Printf("Quota: %d bytes used, no limit\n", me.Quota.Used)
	} else {
		fmt.Printf("Quota: %d of %d bytes used\n", me.Quota.Used, *me.Quota.Limit)
	}
	return nil
}

func (cmd *DriveCmd) readToken() (string, error) {
	var token string
	switch {
	case cmd.tokenStdin:
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read token from stdin: %w", err)
		}
		token = line
	case term.IsTerminal(int(os.Stdin.Fd())):
		fmt.Fprint(os.Stderr, "Token: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read token: %w", err)
		}
		token = string(raw)
	default:
		return "", errors.New("stdin is not a terminal: pass --token-stdin")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("the token is empty")
	}
	return token, nil
}

func (cmd *DriveCmd) pushCmd() *cli.Command {
	return &cli.Command{
		Name:      "push",
		Usage:     "Copy documents to a drive server",
		ArgsUsage: "[paths...]",
		Description: `Uploads markdown and html documents. With no paths, every such document in
the space goes up. A folder path pushes everything under it. A document whose
content matches its last upload is skipped unless --force is set.

The exit status is non-zero when any path fails. With --json, each path is one
object with path, action, sha256, and error.

Example: hive drive push --repo colonyops/hive plans/`,
		Flags: []cli.Flag{
			cmd.serverFlag(),
			cmd.jsonFlag(),
			&cli.BoolFlag{
				Name:        "force",
				Usage:       "upload documents even when unchanged since the last push",
				Destination: &cmd.force,
			},
		},
		Action: cmd.runPush,
	}
}

func (cmd *DriveCmd) runPush(ctx context.Context, c *cli.Command) error {
	remote, space, server, err := cmd.remoteTarget(ctx)
	if err != nil {
		return err
	}
	results, pushErr := remote.Push(ctx, server, space, c.Args().Slice(), cmd.force)
	failed := 0
	for _, r := range results {
		if r.Action == driveremote.ActionFailed {
			failed++
		}
		if err := cmd.printResult(r, r.Action, r.Path, r.Path, r.Error); err != nil {
			return err
		}
	}
	if pushErr != nil {
		return pushErr
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d documents failed", failed, len(results))
	}
	return nil
}

func (cmd *DriveCmd) pullCmd() *cli.Command {
	return &cli.Command{
		Name:      "pull",
		Usage:     "Copy documents from a drive server",
		ArgsUsage: "[paths...]",
		Description: `Downloads documents. With no paths, every document of the space comes down.

Nothing local is overwritten or deleted. A download that differs from the
local document is written beside it as <name>-1.md, <name>-2.md, and so on,
using the first free number.

Example: hive drive pull --repo colonyops/hive`,
		Flags: []cli.Flag{
			cmd.serverFlag(),
			cmd.jsonFlag(),
		},
		Action: cmd.runPull,
	}
}

func (cmd *DriveCmd) runPull(ctx context.Context, c *cli.Command) error {
	remote, space, server, err := cmd.remoteTarget(ctx)
	if err != nil {
		return err
	}
	results, pullErr := remote.Pull(ctx, server, space, c.Args().Slice())
	failed := 0
	for _, r := range results {
		if r.Action == driveremote.ActionFailed {
			failed++
		}
		line := r.Path
		if r.LocalPath != "" {
			line = r.Path + " -> " + r.LocalPath
		}
		if err := cmd.printResult(r, r.Action, line, r.Path, r.Error); err != nil {
			return err
		}
	}
	if pullErr != nil {
		return pullErr
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d documents failed", failed, len(results))
	}
	return nil
}

func (cmd *DriveCmd) shareCmd() *cli.Command {
	return &cli.Command{
		Name:      "share",
		Usage:     "Print a public link to a document on a drive server",
		ArgsUsage: "<path>",
		Description: `Creates a public /s/<slug> link for a document that was pushed. Anyone with
the link can read it. Sharing again returns the same link.

--expires takes 1d, 7d, or 30d. Without it the link does not expire.

Example: hive drive share plans/ship.md --expires 7d`,
		Flags: []cli.Flag{
			cmd.serverFlag(),
			cmd.jsonFlag(),
			&cli.StringFlag{
				Name:        "expires",
				Usage:       "link lifetime: 1d, 7d, or 30d",
				Destination: &cmd.expires,
			},
		},
		Action: cmd.runShare,
	}
}

func (cmd *DriveCmd) runShare(ctx context.Context, c *cli.Command) error {
	remote, space, server, err := cmd.remoteTarget(ctx)
	if err != nil {
		return err
	}
	path := c.Args().First()
	if path == "" || c.Args().Len() > 1 {
		return errors.New("share: exactly one path is required")
	}

	share, err := remote.Share(ctx, server, space, path, cmd.expires)
	if err != nil {
		if cmd.json {
			if jerr := iojson.WriteLine(os.Stdout, shareResult{Path: path, Action: driveremote.ActionFailed, Error: err.Error()}); jerr != nil {
				return jerr
			}
		}
		return err
	}
	if cmd.json {
		return iojson.WriteLine(os.Stdout, shareResult{Path: path, Action: "shared", URL: share.URL, ExpiresAt: share.ExpiresAt})
	}
	fmt.Println(share.URL)
	if share.ExpiresAt != nil {
		fmt.Fprintf(os.Stderr, "Expires %s\n", share.ExpiresAt.Local().Format(time.DateTime))
	}
	return nil
}

type shareResult struct {
	Path      string             `json:"path"`
	Action    driveremote.Action `json:"action"`
	URL       string             `json:"url,omitempty"`
	ExpiresAt *time.Time         `json:"expires_at,omitempty"`
	Error     string             `json:"error,omitempty"`
}

func (cmd *DriveCmd) remoteTarget(ctx context.Context) (*driveremote.Service, drive.Space, string, error) {
	remote, err := cmd.remote()
	if err != nil {
		return nil, drive.Space{}, "", err
	}
	space, err := cmd.space(ctx)
	if err != nil {
		return nil, drive.Space{}, "", err
	}
	server, err := remote.DefaultServer(cmd.server)
	if err != nil {
		return nil, drive.Space{}, "", err
	}
	return remote, space, server, nil
}

func (cmd *DriveCmd) printResult(result any, action driveremote.Action, line, path, errMsg string) error {
	if cmd.json {
		return iojson.WriteLine(os.Stdout, result)
	}
	fmt.Printf("%-9s  %s\n", action, line)
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "  %s: %s\n", path, errMsg)
	}
	return nil
}
