package commands

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/project"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
	"github.com/colonyops/hive/pkg/iojson"
)

// ProjectCmd groups the project session commands. Project sessions run on
// this machine only.
type ProjectCmd struct {
	flags *Flags
	app   *app.App

	project       string
	repos         []string
	cloneStrategy string
	agent         string
	prompt        string
	background    bool
	noSpawn       bool
	json          bool
}

func NewProjectCmd(flags *Flags, app *app.App) *ProjectCmd {
	return &ProjectCmd{flags: flags, app: app}
}

func (cmd *ProjectCmd) Register(root *cli.Command) *cli.Command {
	root.Commands = append(root.Commands, &cli.Command{
		Name:  "project",
		Usage: "Project sessions: one agent over several repositories",
		Commands: []*cli.Command{
			cmd.newCmd(),
			cmd.listCmd(),
			cmd.showCmd(),
			cmd.openCmd(),
			cmd.recycleCmd(),
			cmd.deleteCmd(),
		},
	})
	return root
}

func (cmd *ProjectCmd) jsonFlag() cli.Flag {
	return &cli.BoolFlag{Name: "json", Usage: "output as JSON", Destination: &cmd.json}
}

func (cmd *ProjectCmd) newCmd() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "Check out each repository as its own session and start one agent above them",
		UsageText: "hive project new <name> [--project <definition>] [--repo <remote|path>[=dir]]...",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "project", Aliases: []string{"p"}, Usage: "project definition from config", Destination: &cmd.project},
			&cli.StringSliceFlag{Name: "repo", Aliases: []string{"r"}, Usage: "remote URL or local checkout, optionally =dir", Destination: &cmd.repos},
			&cli.StringFlag{Name: "clone-strategy", Usage: "clone strategy for --repo repositories: full or worktree", Destination: &cmd.cloneStrategy},
			&cli.StringFlag{Name: "agent", Usage: "agent profile", Destination: &cmd.agent},
			&cli.StringFlag{Name: "prompt", Usage: "prompt for the agent", Destination: &cmd.prompt},
			&cli.BoolFlag{Name: "background", Aliases: []string{"bg"}, Usage: "do not attach to tmux", Destination: &cmd.background},
			&cli.BoolFlag{Name: "no-spawn", Usage: "create the checkouts and root without a tmux session", Destination: &cmd.noSpawn},
			cmd.jsonFlag(),
		},
		Action: cmd.runNew,
	}
}

func (cmd *ProjectCmd) runNew(ctx context.Context, c *cli.Command) error {
	name := strings.TrimSpace(strings.Join(c.Args().Slice(), " "))
	if name == "" {
		return fmt.Errorf("a project session needs a name")
	}
	if cmd.app.TargetsServer() {
		return fmt.Errorf("project sessions run on this machine only")
	}

	repos := make([]projectsvc.RepoSpec, 0, len(cmd.repos))
	for _, r := range cmd.repos {
		remote, dir := splitRepoFlag(r)
		repos = append(repos, projectsvc.RepoSpec{Remote: remote, Dir: dir, CloneStrategy: cmd.cloneStrategy})
	}

	ps, err := cmd.app.Projects().Create(ctx, projectsvc.CreateOptions{
		Name:       name,
		Project:    cmd.project,
		Repos:      repos,
		Prompt:     cmd.prompt,
		Agent:      cmd.agent,
		Background: true,
		SkipSpawn:  cmd.noSpawn,
		Progress:   c.Root().ErrWriter,
	})
	if err != nil {
		return err
	}
	if err := cmd.print(ctx, c, ps); err != nil {
		return err
	}
	if cmd.noSpawn || cmd.background || !canAttach() {
		return nil
	}
	return cmd.app.Sessions().AttachOrSwitch(ctx, multiplexer.Target{Session: ps.Slug}, multiplexer.AttachStreams{})
}

// splitRepoFlag splits "remote=dir". A remote never holds "=" before its
// last path segment in practice, so the last "=" wins.
func splitRepoFlag(v string) (remote, dir string) {
	if i := strings.LastIndex(v, "="); i > 0 && !strings.ContainsAny(v[i+1:], "/:") {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func (cmd *ProjectCmd) listCmd() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List project sessions",
		Flags:   []cli.Flag{cmd.jsonFlag()},
		Action: func(ctx context.Context, c *cli.Command) error {
			all, err := cmd.app.Projects().List(ctx)
			if err != nil {
				return err
			}
			out := c.Root().Writer
			if cmd.json {
				for _, ps := range all {
					if err := iojson.WriteLine(out, ps); err != nil {
						return err
					}
				}
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\tNAME\tPROJECT\tREPOS\tROOT")
			for _, ps := range all {
				dirs := make([]string, 0, len(ps.Repos))
				for _, r := range ps.Repos {
					dirs = append(dirs, r.Dir)
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ps.ID, ps.Name, ps.Project, strings.Join(dirs, ","), ps.Root)
			}
			return w.Flush()
		},
	}
}

func (cmd *ProjectCmd) showCmd() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show a project session and its repositories",
		UsageText: "hive project show <id|name>",
		Flags:     []cli.Flag{cmd.jsonFlag()},
		Action: func(ctx context.Context, c *cli.Command) error {
			ps, err := cmd.app.Projects().Resolve(ctx, c.Args().First())
			if err != nil {
				return err
			}
			return cmd.print(ctx, c, ps)
		},
	}
}

type projectOutput struct {
	project.Session
	Children []projectsvc.Child `json:"children"`
}

func (cmd *ProjectCmd) print(ctx context.Context, c *cli.Command, ps project.Session) error {
	children, err := cmd.app.Projects().Children(ctx, ps)
	if err != nil {
		return err
	}
	out := c.Root().Writer
	if cmd.json {
		return iojson.WriteLine(out, projectOutput{Session: ps, Children: children})
	}
	_, _ = fmt.Fprintf(out, "%s (%s)\nroot: %s\ntmux: %s\n", ps.Name, ps.ID, ps.Root, ps.Slug)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "DIR\tSESSION\tSTRATEGY\tBRANCH\tPATH")
	for _, ch := range children {
		if ch.Session.ID == "" {
			_, _ = fmt.Fprintf(w, "%s\t(missing)\t\t\t\n", ch.Dir)
			continue
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ch.Dir, ch.Session.ID, ch.Session.CloneStrategy, ch.Branch, ch.Session.Path)
	}
	return w.Flush()
}

func (cmd *ProjectCmd) openCmd() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "Attach to a project session, starting its tmux session when it is gone",
		UsageText: "hive project open <id|name>",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "background", Aliases: []string{"bg"}, Usage: "do not attach to tmux", Destination: &cmd.background},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return cmd.app.Projects().Open(ctx, c.Args().First(), cmd.background || !canAttach())
		},
	}
}

func (cmd *ProjectCmd) recycleCmd() *cli.Command {
	return &cli.Command{
		Name:      "recycle",
		Usage:     "Recycle every repository's session and remove the project root",
		UsageText: "hive project recycle <id|name>",
		Action: func(ctx context.Context, c *cli.Command) error {
			return cmd.app.Projects().Recycle(ctx, c.Args().First(), c.Root().ErrWriter)
		},
	}
}

func (cmd *ProjectCmd) deleteCmd() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Aliases:   []string{"rm"},
		Usage:     "Delete every repository's session and the project root",
		UsageText: "hive project delete <id|name>",
		Action: func(ctx context.Context, c *cli.Command) error {
			return cmd.app.Projects().Delete(ctx, c.Args().First())
		},
	}
}
