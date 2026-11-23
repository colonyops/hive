package commands

import (
	"context"
	"fmt"
	"slices"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/opscli"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
)

// EngineCmd adds a root command group for each resource of the catalog's
// engine operations: `hive task next` runs task_next in this process.
type EngineCmd struct {
	flags *Flags
	app   *app.App
}

func NewEngineCmd(flags *Flags, app *app.App) *EngineCmd {
	return &EngineCmd{flags: flags, app: app}
}

// Register adds the groups to root. It must run after every root flag is
// declared, because a field flag may not reuse a root flag's name. A group
// whose name a hand-written command already holds, such as todo, gets the
// generated verbs added to it.
func (cmd *EngineCmd) Register(root *cli.Command) *cli.Command {
	reserved := []string{callerFlag}
	for _, f := range root.Flags {
		reserved = append(reserved, f.Names()...)
	}
	groups, err := opscli.EngineCommands(catalog.All(), cmd.run, desktopCaller, reserved...)
	if err != nil {
		panic(err)
	}
	for _, group := range groups {
		for _, leaf := range group.Commands {
			leaf.Metadata = alwaysJSON()
		}
		i := slices.IndexFunc(root.Commands, func(c *cli.Command) bool { return c.Name == group.Name })
		if i < 0 {
			group.Flags = append(group.Flags, engineCallerFlag())
			root.Commands = append(root.Commands, group)
			continue
		}
		existing := root.Commands[i]
		for _, leaf := range group.Commands {
			if find(existing.Commands, leaf.Name) != nil {
				panic(fmt.Sprintf("hive %s %s is both hand-written and generated", group.Name, leaf.Name))
			}
			existing.Commands = append(existing.Commands, leaf)
		}
		if !slices.ContainsFunc(existing.Flags, func(f cli.Flag) bool { return slices.Contains(f.Names(), callerFlag) }) {
			existing.Flags = append(existing.Flags, engineCallerFlag())
		}
	}
	return root
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
