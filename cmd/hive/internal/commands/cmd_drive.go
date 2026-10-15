package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/driveremote"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type DriveCmd struct {
	flags *Flags
	app   *app.App

	repo       string
	workspace  string
	shared     bool
	all        bool
	json       bool
	olderThan  string
	server     string
	force      bool
	tokenStdin bool
	expires    string
}

// NewDriveCmd creates a new drive command.
func NewDriveCmd(flags *Flags, app *app.App) *DriveCmd {
	return &DriveCmd{flags: flags, app: app}
}

// Register adds the drive command to the application.
func (cmd *DriveCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:    "drive",
		Aliases: []string{"ctx"},
		Usage:   "Browse and manage Hive Drive, the documents under the context root",
		Description: `Hive Drive is the document tree under the context root: one space per
repository, per Chats workspace, and one shared space. Agents and people put
markdown, html, and canvas documents there.

The context root is $XDG_DATA_HOME/hive/context, or context.base_dir when set.
A repository's space is {owner}/{repo} under the root. Use 'hive drive init'
in a git repository to create a .hive symlink pointing to its space.

'ctx' is the former name of this command and still works.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "repo",
				Aliases:     []string{"r"},
				Usage:       "target a specific repository space (owner/repo)",
				Destination: &cmd.repo,
			},
			&cli.BoolFlag{
				Name:        "shared",
				Aliases:     []string{"s"},
				Usage:       "use the shared space",
				Destination: &cmd.shared,
			},
			&cli.StringFlag{
				Name:        "workspace",
				Aliases:     []string{"w"},
				Usage:       "target a Chats workspace space by name",
				Destination: &cmd.workspace,
			},
		},
		Commands: []*cli.Command{
			cmd.initCmd(),
			cmd.lsCmd(),
			cmd.spacesCmd(),
			cmd.pruneCmd(),
			cmd.loginCmd(),
			cmd.pushCmd(),
			cmd.pullCmd(),
			cmd.shareCmd(),
		},
	})

	return app
}

func (cmd *DriveCmd) initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create symlink to the space directory",
		Description: `Creates a symlink in the current directory pointing to the space directory.

The symlink name is configured via context.symlink_name (default: .hive).
The target is {owner}/{repo} under the context root ($XDG_DATA_HOME/hive/context,
or context.base_dir when set).`,
		Action: cmd.runInit,
	}
}

func (cmd *DriveCmd) lsCmd() *cli.Command {
	return &cli.Command{
		Name:  "ls",
		Usage: "List the documents and folders of a space",
		Description: `Prints the space directory, then one line per entry: its kind and its path.
Kinds are dir, markdown, html, canvas, and file.

With --all, every space is listed under a header line with its key.
With --json, each entry is one JSON object per line, without children. With
--all and --json, each line is {"space": ..., "entry": ...}.

Example: hive drive ls`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "list every space",
				Destination: &cmd.all,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output one JSON object per line",
				Destination: &cmd.json,
			},
		},
		Action: cmd.runLs,
	}
}

func (cmd *DriveCmd) runLs(ctx context.Context, c *cli.Command) error {
	if cmd.all {
		if cmd.repo != "" || cmd.shared || cmd.workspace != "" {
			return fmt.Errorf("--all cannot be combined with --repo, --shared, or --workspace")
		}
		return cmd.runLsAll(ctx)
	}

	space, err := cmd.space(ctx)
	if err != nil {
		return err
	}
	dir, err := cmd.app.Drive().SpaceDir(space)
	if err != nil {
		return err
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if cmd.json {
			return nil
		}
		fmt.Println("Context directory does not exist. Run 'hive drive init' first.")
		return nil
	}

	root, err := cmd.app.Drive().Tree(ctx, space)
	if err != nil {
		return err
	}
	if cmd.json {
		return writeEntries(root, func(e drive.Entry) any { return e })
	}
	fmt.Println(dir)
	printEntries(root, cmd.uploadStatus(ctx, space))
	return nil
}

// uploadStatus is nil unless a drive server is known, so a machine that never
// logged in sees the plain listing.
func (cmd *DriveCmd) uploadStatus(ctx context.Context, space drive.Space) map[string]driveremote.UploadStatus {
	remote := cmd.app.DriveRemote()
	if remote == nil {
		return nil
	}
	server, err := remote.DefaultServer("")
	if err != nil {
		return nil
	}
	status, err := remote.Status(ctx, server, space)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: upload status unavailable: %v\n", err)
		return nil
	}
	return status
}

func (cmd *DriveCmd) runLsAll(ctx context.Context) error {
	spaces, err := cmd.app.Drive().Spaces(ctx)
	if err != nil {
		return err
	}
	for _, space := range spaces {
		root, err := cmd.app.Drive().Tree(ctx, space)
		if err != nil {
			return err
		}
		if cmd.json {
			err = writeEntries(root, func(e drive.Entry) any {
				return struct {
					Space drive.Space `json:"space"`
					Entry drive.Entry `json:"entry"`
				}{space, e}
			})
			if err != nil {
				return err
			}
			continue
		}
		fmt.Println(space.Key)
		printEntries(root, nil)
	}
	return nil
}

// A non-nil status adds a column for the uploaded or changed documents.
func printEntries(root drive.Entry, status map[string]driveremote.UploadStatus) {
	for _, entry := range root.Children {
		if status == nil {
			fmt.Printf("%-8s  %s\n", entry.Kind, entry.Path)
		} else {
			fmt.Printf("%-8s  %-8s  %s\n", entry.Kind, status[entry.Path], entry.Path)
		}
		printEntries(entry, status)
	}
}

// writeEntries writes every entry under root, depth first, as one JSON line
// each. The entry carries no children: the lines are the tree.
func writeEntries(root drive.Entry, wrap func(drive.Entry) any) error {
	for _, entry := range root.Children {
		children := entry.Children
		entry.Children = nil
		if err := iojson.WriteLine(os.Stdout, wrap(entry)); err != nil {
			return err
		}
		if err := writeEntries(drive.Entry{Children: children}, wrap); err != nil {
			return err
		}
	}
	return nil
}

func (cmd *DriveCmd) spacesCmd() *cli.Command {
	return &cli.Command{
		Name:  "spaces",
		Usage: "List the spaces under the context root",
		Description: `Prints one line per space: its kind and its key. The shared space is always listed.
With --json, each space is one JSON object per line.

Example: hive drive spaces`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output one JSON object per line",
				Destination: &cmd.json,
			},
		},
		Action: cmd.runSpaces,
	}
}

func (cmd *DriveCmd) runSpaces(ctx context.Context, c *cli.Command) error {
	spaces, err := cmd.app.Drive().Spaces(ctx)
	if err != nil {
		return err
	}
	for _, space := range spaces {
		if cmd.json {
			if err := iojson.WriteLine(os.Stdout, space); err != nil {
				return err
			}
			continue
		}
		fmt.Printf("%-10s  %s\n", space.Kind, space.Key)
	}
	return nil
}

func (cmd *DriveCmd) pruneCmd() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "Delete old files from a space",
		Description: `Deletes files older than the specified duration.

Example: hive drive prune --older-than 7d`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "older-than",
				Usage:       "delete files older than this duration (e.g., 7d, 24h)",
				Destination: &cmd.olderThan,
				Required:    true,
			},
		},
		Action: cmd.runPrune,
	}
}

func (cmd *DriveCmd) runInit(ctx context.Context, c *cli.Command) error {
	ctxDir, err := cmd.resolveContextDir(ctx)
	if err != nil {
		return err
	}

	createdSubdirs, err := cmd.app.Drive().Init(ctxDir)
	if err != nil {
		return err
	}

	alreadyExists, err := cmd.app.Drive().CreateSymlink(ctxDir)
	if err != nil {
		return err
	}

	symlinkName := cmd.app.Config.Context.SymlinkName
	if alreadyExists {
		fmt.Fprintf(os.Stderr, "Symlink already exists: %s -> %s\n", symlinkName, ctxDir)
	} else {
		fmt.Fprintf(os.Stderr, "Created symlink: %s -> %s\n", symlinkName, ctxDir)
	}

	if len(createdSubdirs) > 0 {
		fmt.Fprintf(os.Stderr, "Created subdirectories: %s\n", strings.Join(createdSubdirs, ", "))
	}
	return nil
}

func (cmd *DriveCmd) runPrune(ctx context.Context, c *cli.Command) error {
	ctxDir, err := cmd.resolveContextDir(ctx)
	if err != nil {
		return err
	}

	duration, err := parseDuration(cmd.olderThan)
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}

	count, err := cmd.app.Drive().Prune(ctxDir, duration)
	if err != nil {
		return err
	}

	if count == 0 {
		fmt.Fprintf(os.Stderr, "No files older than %s\n", cmd.olderThan)
	} else {
		fmt.Fprintf(os.Stderr, "Removed %d file(s) older than %s\n", count, cmd.olderThan)
	}
	return nil
}

func (cmd *DriveCmd) resolveContextDir(ctx context.Context) (string, error) {
	space, err := cmd.space(ctx)
	if err != nil {
		return "", err
	}
	return cmd.app.Drive().SpaceDir(space)
}

// space is the space a command targets: --workspace, else what ResolveSpace
// makes of --repo, --shared, and the current directory.
func (cmd *DriveCmd) space(ctx context.Context) (drive.Space, error) {
	if cmd.workspace == "" {
		return cmd.app.Drive().ResolveSpace(ctx, cmd.repo, cmd.shared)
	}
	if cmd.repo != "" || cmd.shared {
		return drive.Space{}, errors.New("--workspace cannot be combined with --repo or --shared")
	}
	return drive.WorkspaceSpace(cmd.workspace)
}

func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix (not supported by time.ParseDuration)
	if before, ok := strings.CutSuffix(s, "d"); ok {
		days := before
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid days: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
