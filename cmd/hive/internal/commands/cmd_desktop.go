package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/deeplink"
	"github.com/colonyops/hive/internal/domain/session"
	hivesession "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/platform/git"
)

// DesktopCmd implements the hive desktop command group.
type DesktopCmd struct {
	flags *Flags
	app   *app.App

	window string
	canvas string
	feed   string
	owner  string
}

// NewDesktopCmd creates a new desktop command group.
func NewDesktopCmd(flags *Flags, app *app.App) *DesktopCmd {
	return &DesktopCmd{flags: flags, app: app}
}

// Register adds the desktop command group to the application.
func (cmd *DesktopCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:    "desktop",
		Aliases: []string{"dt"},
		Usage:   "Link into Hive Desktop",
		Description: `Builds and opens hive:// links, which Hive Desktop registers with the operating
system. A link opens a Code session, a chat, an inbox item, or a canvas.

  hive://code/<slug>[?window=<id>][&canvas=<name>]
  hive://chats/<workspace>[/<chat>][?canvas=<name>]
  hive://inbox/<profile>[/<item>][?feed=<id>]
  hive://canvas/<name>?owner=<workspace | owner/repo | @global>`,
		Commands: []*cli.Command{
			cmd.urlCmd(),
			cmd.openCmd(),
		},
	})
	return app
}

func (cmd *DesktopCmd) urlCmd() *cli.Command {
	return &cli.Command{
		Name:  "url",
		Usage: "Print the hive:// link to a place in Hive Desktop",
		Description: `Prints a link without opening it, for another program to put behind a button.

Examples:
  hive dt url code                       # the hive session for this directory
  hive dt url code my-tmux-session --window @2
  hive dt url canvas plan                # this repository's canvas`,
		Commands: cmd.locationCommands(cmd.printURL),
	}
}

func (cmd *DesktopCmd) openCmd() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "Open a place in Hive Desktop",
		UsageText: "hive desktop open <hive://...>\nhive desktop open <code|chat|inbox|canvas> ...",
		Description: `Opens a hive:// link through the operating system, which starts Hive Desktop if
it is not running. Pass a link, or the same arguments 'hive desktop url' takes.`,
		Commands: cmd.locationCommands(cmd.openLocation),
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.Args().Len() != 1 {
				return cli.ShowSubcommandHelp(c)
			}
			loc, err := deeplink.Parse(c.Args().First())
			if err != nil {
				return err
			}
			return cmd.openLocation(ctx, c, loc)
		},
	}
}

// locationCommands is the one argument grammar url and open share, so a link
// printed for later and a link opened now are always the same link.
func (cmd *DesktopCmd) locationCommands(run func(context.Context, *cli.Command, deeplink.Location) error) []*cli.Command {
	canvasFlag := &cli.StringFlag{
		Name:        "canvas",
		Usage:       "open the canvas pane beside it, on this canvas",
		Destination: &cmd.canvas,
	}

	return []*cli.Command{
		{
			Name:      "code",
			Usage:     "A tmux session in Code",
			ArgsUsage: "[slug]",
			Description: `Names a tmux session by its name. Without one, it names the hive session that
holds the current directory.`,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "window",
					Usage:       "tmux window id to select, like @3",
					Destination: &cmd.window,
				},
				canvasFlag,
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				if c.Args().Len() > 1 {
					return errors.New("code takes at most one slug")
				}
				slug := c.Args().First()
				if slug == "" {
					sess, err := cmd.currentSession(ctx)
					if err != nil {
						return err
					}
					slug = hivesession.Target(sess).Session
				}
				return run(ctx, c, deeplink.Location{Area: deeplink.AreaCode, Slug: slug, Window: cmd.window, Canvas: cmd.canvas})
			},
		},
		{
			Name:      "chat",
			Aliases:   []string{"chats"},
			Usage:     "A workspace in Chats, or one chat in it",
			ArgsUsage: "<workspace> [chat-id]",
			Flags:     []cli.Flag{canvasFlag},
			Action: func(ctx context.Context, c *cli.Command) error {
				if c.Args().Len() < 1 || c.Args().Len() > 2 {
					return errors.New("chat takes a workspace and an optional chat id")
				}
				return run(ctx, c, deeplink.Location{
					Area:      deeplink.AreaChats,
					Workspace: c.Args().Get(0),
					Chat:      c.Args().Get(1),
					Canvas:    cmd.canvas,
				})
			},
		},
		{
			Name:      "inbox",
			Usage:     "A profile's inbox, or one item in it",
			ArgsUsage: "<profile> [item-id]",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "feed",
					Usage:       "feed to open",
					Destination: &cmd.feed,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				if c.Args().Len() < 1 || c.Args().Len() > 2 {
					return errors.New("inbox takes a profile and an optional item id")
				}
				loc := deeplink.Location{Area: deeplink.AreaInbox, Profile: c.Args().Get(0), Feed: cmd.feed}
				if raw := c.Args().Get(1); raw != "" {
					item, err := strconv.ParseInt(raw, 10, 64)
					if err != nil || item <= 0 {
						return fmt.Errorf("item id %q is not a positive number", raw)
					}
					loc.Item = item
				}
				return run(ctx, c, loc)
			},
		},
		{
			Name:      "canvas",
			Usage:     "A canvas in the full-page canvas view",
			ArgsUsage: "<name>",
			Description: `The owner defaults to the repository of the hive session that holds the current
directory.`,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "owner",
					Usage:       "canvas owner: a Chats workspace, owner/repo, or " + deeplink.GlobalCanvasOwner,
					Destination: &cmd.owner,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				if c.Args().Len() != 1 {
					return errors.New("canvas takes one canvas name")
				}
				owner := cmd.owner
				if owner == "" {
					var err error
					if owner, err = cmd.currentRepository(ctx); err != nil {
						return err
					}
				}
				return run(ctx, c, deeplink.Location{Area: deeplink.AreaCanvas, Canvas: c.Args().First(), Owner: owner})
			},
		},
	}
}

func (cmd *DesktopCmd) printURL(_ context.Context, c *cli.Command, loc deeplink.Location) error {
	link, err := loc.URL()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.Root().Writer, link)
	return err
}

func (cmd *DesktopCmd) openLocation(ctx context.Context, _ *cli.Command, loc deeplink.Location) error {
	link, err := loc.URL()
	if err != nil {
		return err
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	out, err := exec.CommandContext(ctx, opener, link).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s %s: %w: %s", opener, link, err, msg)
		}
		return fmt.Errorf("%s %s: %w", opener, link, err)
	}
	return nil
}

func (cmd *DesktopCmd) currentSession(ctx context.Context) (session.Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return session.Session{}, fmt.Errorf("get working directory: %w", err)
	}
	sess, ok, err := cmd.app.Sessions().SessionAtPath(ctx, cwd)
	if err != nil {
		return session.Session{}, fmt.Errorf("detect session: %w", err)
	}
	if !ok {
		return session.Session{}, errors.New("not in a hive session; name one explicitly")
	}
	return sess, nil
}

// currentRepository answers the way the desktop's canvas server does for a
// working directory: the repository of the hive session that holds it.
func (cmd *DesktopCmd) currentRepository(ctx context.Context) (string, error) {
	sess, err := cmd.currentSession(ctx)
	if err != nil {
		return "", fmt.Errorf("%w (or pass --owner)", err)
	}
	owner, repo := git.ExtractOwnerRepo(sess.Remote)
	if owner == "" || repo == "" {
		return "", fmt.Errorf("session remote %q names no owner/repo; pass --owner", sess.Remote)
	}
	return owner + "/" + repo, nil
}
