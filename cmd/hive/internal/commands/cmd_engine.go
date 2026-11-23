package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/opscli"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/ops/tasks"
)

// engineNamespaces puts each engine resource below the command group that
// already holds it: task_comment.create is hive hc comment create.
var engineNamespaces = map[string][]string{
	"task":    {"hc"},
	"message": {"msg"},
	"topic":   {"msg", "topic"},
	"todo":    {"todo"},
}

var engineAliases = map[string][]string{
	"task.list":      {"ls"},
	"task.get":       {"show"},
	"task.clear":     {"prune"},
	"message.create": {"pub"},
	"todo.create":    {"add"},
}

func enginePath(info ops.Info) []string {
	p := opscli.Path(info)
	ns, ok := engineNamespaces[p[0]]
	if !ok {
		panic(fmt.Sprintf("engine resource %s has no namespace in engineNamespaces", info.Resource))
	}
	return append(slices.Clone(ns), p[1:]...)
}

// EngineCmd adds the catalog's engine operations to the hc, msg and todo
// groups. They run in this process.
type EngineCmd struct {
	flags *Flags
	app   *app.App
}

func NewEngineCmd(flags *Flags, app *app.App) *EngineCmd {
	return &EngineCmd{flags: flags, app: app}
}

// Register merges the generated commands into root. It must run after every
// root flag is declared, because a field flag may not reuse a root flag's
// name, and after the hand-written groups it adds to.
func (cmd *EngineCmd) Register(root *cli.Command) *cli.Command {
	reserved := []string{callerFlag}
	for _, f := range root.Flags {
		reserved = append(reserved, f.Names()...)
	}
	groups, err := opscli.EngineCommands(catalog.All(), opscli.Engine{
		Run:      cmd.run,
		Print:    cmd.print,
		Path:     enginePath,
		Aliases:  engineAliases,
		Caller:   desktopCaller,
		Reserved: reserved,
	})
	if err != nil {
		panic(err)
	}
	for _, group := range groups {
		markJSON(group)
		existing := find(root.Commands, group.Name)
		if existing == nil {
			panic(fmt.Sprintf("no hand-written hive %s group for the engine ops", group.Name))
		}
		merge(existing, group)
		if !slices.ContainsFunc(existing.Flags, func(f cli.Flag) bool { return slices.Contains(f.Names(), callerFlag) }) {
			existing.Flags = append(existing.Flags, engineCallerFlag())
		}
	}
	return root
}

func markJSON(c *cli.Command) {
	if c.Action != nil {
		c.Metadata = alwaysJSON()
	}
	for _, sub := range c.Commands {
		markJSON(sub)
	}
}

// merge adds the generated commands of from to into, joining groups of the
// same name.
func merge(into, from *cli.Command) {
	for _, c := range from.Commands {
		existing := find(into.Commands, c.Name)
		switch {
		case existing == nil:
			into.Commands = append(into.Commands, c)
		case len(c.Commands) > 0 && len(existing.Commands) > 0:
			merge(existing, c)
		default:
			panic(fmt.Sprintf("hive %s %s is both hand-written and generated", into.Name, c.Name))
		}
	}
}

func engineCallerFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    callerFlag,
		Usage:   "Who the call acts for: a session id, a chat id, a path, or global (default: $HIVE_AGENT_SESSION, else the current directory)",
		Sources: cli.EnvVars("HIVE_AGENT_SESSION"),
	}
}

func (cmd *EngineCmd) run(ctx context.Context, _ *cli.Command, info ops.Info, args map[string]any) (any, error) {
	return callEngine(ctx, cmd.app, info.Name, args)
}

// print renders a task list, a task or an epic context for a person at a
// terminal, and writes JSON Lines everywhere else.
func (cmd *EngineCmd) print(ctx context.Context, c *cli.Command, _ ops.Info, out any) error {
	w := c.Root().Writer
	if !isTerminal(w) {
		return opscli.Print(w, out)
	}
	switch out := out.(type) {
	case tasks.TaskListOut:
		renderColoredTree(w, out.Tasks)
	case tasks.TaskGetOut:
		cmd.renderTask(ctx, w, out)
	case hc.ContextBlock:
		renderMarkdown(w, out.String())
	default:
		return opscli.Print(w, out)
	}
	return nil
}

func (cmd *EngineCmd) renderTask(ctx context.Context, w io.Writer, out tasks.TaskGetOut) {
	item := out.Task
	var epicTitle string
	if item.EpicID != "" {
		if epic, err := cmd.app.HC().GetItem(ctx, item.EpicID); err != nil {
			cmd.app.Logger.Debug().Err(err).Str("epic_id", item.EpicID).Msg("failed to resolve epic title")
		} else {
			epicTitle = epic.Title
		}
	}
	var blockers []hc.Item
	for _, id := range item.BlockerIDs {
		b, err := cmd.app.HC().GetItem(ctx, id)
		if err != nil {
			cmd.app.Logger.Debug().Err(err).Str("blocker_id", id).Msg("failed to fetch blocker item")
			continue
		}
		blockers = append(blockers, b)
	}
	renderItem(w, item, out.Comments, epicTitle, blockers)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd())) //nolint:gosec // a file descriptor fits in int
}

// callEngine runs the named engine operation in this process.
func callEngine(ctx context.Context, a *app.App, name string, args map[string]any) (any, error) {
	for _, op := range engineops.New(a.Logger, a.Engine).Ops() {
		if op.Spec.Info().Name == name {
			return op.Call(ctx, args)
		}
	}
	return nil, fmt.Errorf("no engine operation %s", name)
}

func find(cmds []*cli.Command, name string) *cli.Command {
	for _, c := range cmds {
		if c.Name == name {
			return c
		}
	}
	return nil
}
