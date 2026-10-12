package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/urfave/cli/v3"
)

type CtxCmd struct {
	flags *Flags
	app   *app.App

	// Shared flags
	repo   string
	shared bool

	// prune flag
	olderThan string
}

// NewCtxCmd creates a new ctx command.
func NewCtxCmd(flags *Flags, app *app.App) *CtxCmd {
	return &CtxCmd{flags: flags, app: app}
}

// Register adds the ctx command to the application.
func (cmd *CtxCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "ctx",
		Usage: "Manage context directories for inter-agent communication",
		Description: `Context commands manage shared directories for agents.

Each repository gets its own context directory at $XDG_DATA_HOME/hive/context/{owner}/{repo}/.
Use 'hive ctx init' in a git repository to create a .hive symlink pointing to this directory.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "repo",
				Aliases:     []string{"r"},
				Usage:       "target a specific repository context (owner/repo)",
				Destination: &cmd.repo,
			},
			&cli.BoolFlag{
				Name:        "shared",
				Aliases:     []string{"s"},
				Usage:       "use the shared context directory",
				Destination: &cmd.shared,
			},
		},
		Commands: []*cli.Command{
			cmd.initCmd(),
			cmd.lsCmd(),
			cmd.pruneCmd(),
		},
	})

	return app
}

func (cmd *CtxCmd) initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create symlink to context directory",
		Description: `Creates a symlink in the current directory pointing to the context directory.

The symlink name is configured via context.symlink_name (default: .hive).
The target is $XDG_DATA_HOME/hive/context/{owner}/{repo}/.`,
		Action: cmd.runInit,
	}
}

func (cmd *CtxCmd) lsCmd() *cli.Command {
	return &cli.Command{
		Name:  "ls",
		Usage: "List context directory contents as a tree",
		Description: `Displays the context directory contents in a tree view.

Example: hive ctx ls`,
		Action: cmd.runLs,
	}
}

func (cmd *CtxCmd) runLs(ctx context.Context, c *cli.Command) error {
	space, err := cmd.app.Drive().ResolveSpace(ctx, cmd.repo, cmd.shared)
	if err != nil {
		return err
	}
	dir, err := cmd.app.Drive().SpaceDir(space)
	if err != nil {
		return err
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Println("Context directory does not exist. Run 'hive ctx init' first.")
		return nil
	}

	root, err := cmd.app.Drive().Tree(ctx, space)
	if err != nil {
		return err
	}
	fmt.Println(dir)
	printEntries(root)
	return nil
}

// The kind column is as wide as the longest kind name, markdown.
func printEntries(root drive.Entry) {
	for _, entry := range root.Children {
		fmt.Printf("%-8s  %s\n", entry.Kind, entry.Path)
		printEntries(entry)
	}
}

func (cmd *CtxCmd) pruneCmd() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "Delete old files from context directory",
		Description: `Deletes files older than the specified duration.

Example: hive ctx prune --older-than 7d`,
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

func (cmd *CtxCmd) runInit(ctx context.Context, c *cli.Command) error {
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

func (cmd *CtxCmd) runPrune(ctx context.Context, c *cli.Command) error {
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

func (cmd *CtxCmd) resolveContextDir(ctx context.Context) (string, error) {
	return cmd.app.Drive().ResolveDir(ctx, cmd.repo, cmd.shared)
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
