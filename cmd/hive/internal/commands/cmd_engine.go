package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/opscli"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
	driveops "github.com/colonyops/hive/internal/ops/drive"
	"github.com/colonyops/hive/internal/ops/sessions"
	"github.com/colonyops/hive/internal/ops/tasks"
)

// engineNamespaces puts each engine resource below the command group that
// already holds it: task_comment.create is hive hc comment create.
var engineNamespaces = map[string][]string{
	"task":     {"hc"},
	"message":  {"msg"},
	"topic":    {"msg", "topic"},
	"todo":     {"todo"},
	"document": {"drive"},
	"session":  {"session"},
}

var engineAliases = map[string][]string{
	"task.list":      {"ls"},
	"task.get":       {"show"},
	"task.clear":     {"prune"},
	"message.create": {"pub"},
	"todo.create":    {"add"},
	"session.list":   {"ls"},
	"session.get":    {"show"},
}

// jsonFlag is the switch the hand-written session commands had. The
// generated ones keep it hidden: it forces JSON at a terminal.
const jsonFlag = "json"

// legacyAliases keep the short spellings of the hand-written session create
// flags, by op name and field flag, so a script that used them still runs.
var legacyAliases = map[string]map[string][]string{
	"session.create": {"remote": {"r"}, "source": {"s"}, "agent": {"a"}, "background": {"bg"}},
}

func enginePath(info ops.Info) []string {
	p := opscli.Path(info)
	ns, ok := engineNamespaces[p[0]]
	if !ok {
		panic(fmt.Sprintf("engine resource %s has no namespace in engineNamespaces", info.Resource))
	}
	return append(slices.Clone(ns), p[1:]...)
}

// EngineCmd adds the catalog's engine operations to the groups
// engineNamespaces names. They run in this process.
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
		JoinRest: map[string]bool{sessions.SessionCreate.Name: true},
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
	keepSessionSpellings(root)
	addServerFlags(root)
	return root
}

// addServerFlags gives every generated engine command outside the session
// toolsets the --server of the session commands, so --server picks the
// server the same way for every engine operation.
func addServerFlags(root *cli.Command) {
	for _, op := range catalog.All() {
		info := op.Info()
		if info.Owner != ops.OwnerEngine || isSessionOp(info) {
			continue
		}
		c := lookupPath(root, enginePath(info))
		if c == nil {
			panic(fmt.Sprintf("no command for %s", info.Name))
		}
		if slices.ContainsFunc(c.Flags, func(f cli.Flag) bool { return slices.Contains(f.Names(), serverFlagName) }) {
			panic(fmt.Sprintf("hive %s already has --%s", strings.Join(enginePath(info), " "), serverFlagName))
		}
		c.Flags = append(c.Flags, &cli.StringFlag{
			Name:    serverFlagName,
			Usage:   "run on the named server under servers: (local is this machine)",
			Sources: cli.EnvVars("HIVE_SERVER"),
		})
	}
}

func isSessionOp(info ops.Info) bool {
	return info.Toolset == sessions.Toolset || info.Toolset == sessions.ControlToolset
}

// keepSessionSpellings gives the generated session commands the spellings of
// the hand-written ones they replaced: a hidden --json, HIVE_SERVER for
// --server, the short create flags, and the hidden root hive ls.
func keepSessionSpellings(root *cli.Command) {
	for _, op := range catalog.All() {
		info := op.Info()
		if info.Toolset != sessions.Toolset && info.Toolset != sessions.ControlToolset {
			continue
		}
		c := lookupPath(root, enginePath(info))
		if c == nil {
			panic(fmt.Sprintf("no command for %s", info.Name))
		}
		c.Flags = append(c.Flags, &cli.BoolFlag{Name: jsonFlag, Usage: "write JSON even at a terminal", Hidden: true})
		for _, f := range c.Flags {
			switch f := f.(type) {
			case *cli.StringFlag:
				if f.Name == serverFlagName {
					f.Sources = cli.EnvVars("HIVE_SERVER")
				}
				f.Aliases = append(f.Aliases, legacyAliases[info.Name][f.Name]...)
			case *cli.BoolFlag:
				f.Aliases = append(f.Aliases, legacyAliases[info.Name][f.Name]...)
			}
		}
	}
	list := lookupPath(root, enginePath(sessions.SessionList.Info()))
	root.Commands = append(root.Commands, &cli.Command{
		Name:     "ls",
		Usage:    "List all sessions (alias for 'session list')",
		Hidden:   true,
		Flags:    list.Flags,
		Before:   list.Before,
		Action:   list.Action,
		After:    list.After,
		Metadata: list.Metadata,
	})
}

func lookupPath(c *cli.Command, path []string) *cli.Command {
	for _, name := range path {
		if c = find(c.Commands, name); c == nil {
			return nil
		}
	}
	return c
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

func (cmd *EngineCmd) run(ctx context.Context, c *cli.Command, info ops.Info, args map[string]any) (any, error) {
	server := ""
	if !isSessionOp(info) {
		server = c.String(serverFlagName)
	}
	remote := server != "" && server != session.LocalServer
	if remote {
		dropLocalCaller(args)
	}
	if info.Toolset == driveops.Toolset {
		if err := refuseSpaceFlags(c); err != nil {
			return nil, err
		}
		if !remote && cmd.isChat(ctx, args) {
			return cmd.forward(ctx, info, args)
		}
	}
	if info.Name == sessions.SessionCreate.Name {
		return cmd.createSession(ctx, c, args)
	}
	return callEngineOn(ctx, cmd.app, info.Name, server, args, c.Root().ErrWriter)
}

// dropLocalCaller makes a caller that came from HIVE_AGENT_SESSION global:
// that id names a session or chat on this machine, not on a server.
func dropLocalCaller(args map[string]any) {
	env := strings.TrimSpace(os.Getenv(sessionsvc.AgentSessionEnv))
	if caller, ok := args[ops.CallerField].(string); ok && env != "" && caller == env {
		args[ops.CallerField] = sessionsvc.CallerGlobal
	}
}

// createSession runs hive session create like hive new: from the working
// directory, and attached afterwards unless --background is set.
func (cmd *EngineCmd) createSession(ctx context.Context, c *cli.Command, args map[string]any) (any, error) {
	if explicit, _ := args[ops.CallerField].(string); !c.IsSet(callerFlag) || explicit == os.Getenv("HIVE_AGENT_SESSION") {
		delete(args, ops.CallerField)
	}
	created, err := createSession(ctx, cmd.app, args, c.Root().ErrWriter)
	if err != nil {
		return nil, err
	}
	if background, _ := args["background"].(bool); !background {
		if err := attachCreated(ctx, cmd.app, created, c.Root().ErrWriter); err != nil {
			return nil, err
		}
	}
	return created, nil
}

// refuseSpaceFlags rejects the hive drive flags that pick a space by name. A
// document operation files under its caller's space, and a flag that parsed
// but did nothing would write somewhere the user did not ask for.
func refuseSpaceFlags(c *cli.Command) error {
	for _, name := range []string{"repo", "shared", "workspace"} {
		if c.IsSet(name) {
			return invalidf("--%s picks a space for hive drive ls and the other listing commands; this command files under its caller's space, so pass --caller instead", name)
		}
	}
	return nil
}

// isChat reports whether a document call comes from a chat. Chat records live
// in the desktop's store, so only the running desktop can file its documents.
func (cmd *EngineCmd) isChat(ctx context.Context, args map[string]any) bool {
	value, _ := args["caller"].(string)
	c, err := cmd.app.Sessions().ResolveCaller(ctx, value, "", "")
	return err == nil && c.Kind == sessionsvc.CallerKindChat
}

// forward runs a document call on the running desktop, as hive dt does.
func (cmd *EngineCmd) forward(ctx context.Context, info ops.Info, args map[string]any) (any, error) {
	res, err := NewDesktopClient(cmd.app.Config.DataDir, "", cmd.app.Build.Version).Call(ctx, info.Toolset.MCPPath, info.ToolName, args)
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, toolError(res, info.ToolName)
	}
	if info.Name != driveops.DocumentList.Name {
		return toolResultValue(res), nil
	}
	var out driveops.DocumentListOut
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return nil, fmt.Errorf("encode %s result: %w", info.ToolName, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode %s result: %w", info.ToolName, err)
	}
	return out, nil
}

// print renders a task list, a task or an epic context for a person at a
// terminal, and writes JSON Lines everywhere else.
func (cmd *EngineCmd) print(ctx context.Context, c *cli.Command, info ops.Info, out any) error {
	w := c.Root().Writer
	if info.Toolset == sessions.Toolset || info.Toolset == sessions.ControlToolset {
		return cmd.printSessionOp(c, info, out, isTerminal(w) && !c.Bool(jsonFlag))
	}
	if !isTerminal(w) {
		return opscli.Print(w, out)
	}
	switch out := out.(type) {
	case tasks.TaskListOut:
		renderColoredTree(w, out.Tasks)
	case tasks.TaskGetOut:
		cmd.renderTask(ctx, w, out, c.String(serverFlagName))
	case hc.ContextBlock:
		renderMarkdown(w, out.String())
	default:
		return opscli.Print(w, out)
	}
	return nil
}

// renderTask looks up the task's epic and blockers on the server that holds
// the task.
func (cmd *EngineCmd) renderTask(ctx context.Context, w io.Writer, out tasks.TaskGetOut, server string) {
	item := out.Task
	getItem := cmd.app.HC().GetItem
	if server != "" && server != session.LocalServer {
		getItem = func(ctx context.Context, id string) (hc.Item, error) {
			got, err := callEngineOn(ctx, cmd.app, tasks.TaskGet.Name, server, map[string]any{"taskId": id}, io.Discard)
			if err != nil {
				return hc.Item{}, err
			}
			return got.(tasks.TaskGetOut).Task, nil
		}
	}
	var epicTitle string
	if item.EpicID != "" {
		if epic, err := getItem(ctx, item.EpicID); err != nil {
			cmd.app.Logger.Debug().Err(err).Str("epic_id", item.EpicID).Msg("failed to resolve epic title")
		} else {
			epicTitle = epic.Title
		}
	}
	var blockers []hc.Item
	for _, id := range item.BlockerIDs {
		b, err := getItem(ctx, id)
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

// callEngine runs the named engine operation in this process. Session
// operations reach every server through a.Servers, and write their progress
// to progress.
func callEngine(ctx context.Context, a *app.App, name string, args map[string]any, progress io.Writer) (any, error) {
	return callEngineOn(ctx, a, name, "", args, progress)
}

// callEngineOn is callEngine on the named server, or on the server of a
// qualified caller when server is empty.
func callEngineOn(ctx context.Context, a *app.App, name, server string, args map[string]any, progress io.Writer) (any, error) {
	fleet := engineops.WithFleet(func() *servers.Fleet { return a.Servers })
	handlers := engineops.New(a.Logger, a.Engine, fleet, engineops.WithProgress(progress))
	for _, op := range handlers.Ops() {
		if op.Spec.Info().Name == name {
			return handlers.CallOn(ctx, op, server, args)
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
