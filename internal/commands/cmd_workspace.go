package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/colonyops/hive/internal/core/workspace"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

// WorkspaceCmd groups workspace management subcommands.
type WorkspaceCmd struct {
	flags  *Flags
	app    *hive.App
	lsJSON bool
}

// NewWorkspaceCmd creates a new workspace command.
func NewWorkspaceCmd(flags *Flags, app *hive.App) *WorkspaceCmd {
	return &WorkspaceCmd{flags: flags, app: app}
}

// Register adds the workspace command group to the CLI.
func (cmd *WorkspaceCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "workspace",
		Usage: "Workspace management commands",
		Commands: []*cli.Command{
			cmd.listCmd(),
		},
	})
	return app
}

func (cmd *WorkspaceCmd) listCmd() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List discovered repositories from configured workspaces",
		UsageText: "hive workspace list [--json]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON lines",
				Destination: &cmd.lsJSON,
			},
		},
		Action: cmd.runList,
	}
}

// workspaceRepoOutput is the JSON output format for workspace list.
type workspaceRepoOutput struct {
	Name   string `json:"name"`
	Remote string `json:"remote"`
	Path   string `json:"path"`
}

func (cmd *WorkspaceCmd) runList(ctx context.Context, c *cli.Command) error {
	dirs := cmd.app.Config.Workspaces
	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "No workspaces configured. Add workspace directories to your config.")
		return nil
	}

	repos, err := workspace.ScanRepoDirs(ctx, dirs, cmd.app.Sessions.Git())
	if err != nil {
		return fmt.Errorf("scan workspaces: %w", err)
	}

	out := c.Root().Writer

	if cmd.lsJSON {
		for _, r := range repos {
			if err := iojson.WriteLine(out, workspaceRepoOutput{
				Name:   r.Name,
				Remote: r.Remote,
				Path:   r.Path,
			}); err != nil {
				return err
			}
		}
		return nil
	}

	if len(repos) == 0 {
		_, _ = fmt.Fprintln(out, "No repositories found in configured workspaces.")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tREMOTE\tPATH")
	for _, r := range repos {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Remote, r.Path)
	}
	return tw.Flush()
}
