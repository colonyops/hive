package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/hive/servers"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/ops/sessions"
	"github.com/colonyops/hive/pkg/logutils"
)

const toolsetFlag = "toolset"

// MCPCmd implements the hive mcp command group.
type MCPCmd struct {
	flags *Flags
	app   *app.App
}

// NewMCPCmd creates a new mcp command group.
func NewMCPCmd(flags *Flags, app *app.App) *MCPCmd {
	return &MCPCmd{flags: flags, app: app}
}

// Register adds the mcp command group to the application.
func (cmd *MCPCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "mcp",
		Usage: "Serve Hive operations to agents over MCP",
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Serve Hive's tools over stdio",
				Description: `Runs an MCP server on stdin and stdout, so an agent needs no port. Add it to
the agent's config:

  Claude Code: claude mcp add hive -- hive mcp serve --toolset tasks,messages
  Codex:       codex mcp add hive -- hive mcp serve --toolset tasks,messages

The tools are the engine operations behind "hive hc", "hive msg", "hive todo",
"hive session" and "hive drive", and the desktop tools "hive dt" calls.
--toolset picks which (default: all).

Engine operations run in this process on hive.db in --data-dir, so they work
while Hive Desktop is closed. The database opens on the first such call and
stays open. Session create, start, recycle and delete (the session-control
toolset) and the desktop tools go to the running desktop. The server finds it
through the endpoint file on every call, so a desktop restart needs no
reconnect. While the desktop is not running, those calls answer
"unavailable: Hive Desktop is not running".`,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:  toolsetFlag,
						Usage: "Toolsets to serve, comma separated (default: all)",
					},
					&cli.StringFlag{
						Name:    callerFlag,
						Usage:   "Who the calls act for (default: $HIVE_AGENT_SESSION, else the current directory)",
						Sources: cli.EnvVars("HIVE_AGENT_SESSION"),
					},
					&cli.StringFlag{
						Name:    desktopURLFlag,
						Usage:   "Base URL of Hive Desktop (default: $HIVE_DESKTOP_URL, else the running desktop)",
						Sources: cli.EnvVars("HIVE_DESKTOP_URL"),
					},
				},
				Action: cmd.runServe,
			},
		},
	})
	return app
}

func (cmd *MCPCmd) runServe(ctx context.Context, c *cli.Command) error {
	caller, err := resolveCaller(c.String(callerFlag))
	if err != nil {
		return err
	}
	client := NewDesktopClient(cmd.app.Config.DataDir, c.String(desktopURLFlag), cmd.app.Build.Version)
	log := logutils.Component(cmd.app.Logger, "cli.mcp")
	engine := newLocalEngine(ctx, cmd.openEngine, cmd.app.Build.Version, log)
	srv, err := newMCPServer(client, engine, caller, splitToolsets(c.StringSlice(toolsetFlag)), cmd.app.Build.Version, log)
	if err != nil {
		return err
	}
	log.Info().Str("caller", caller).Msg("serving MCP on stdio")
	// The protocol owns stdout. Logs go to the log file, so nothing else may
	// print there while this runs.
	return errors.Join(srv.Run(ctx, &mcp.StdioTransport{}), engine.Close())
}

// openEngine composes the engine handlers as the CLI's session commands do:
// session reads go through the daemon when experimental.daemon is on, and
// through every server under servers:.
func (cmd *MCPCmd) openEngine(ctx context.Context) (*engineops.Handlers, func() error, error) {
	if cmd.app.OpenEngine == nil {
		return nil, nil, errors.New("this hive has no engine to open")
	}
	rt, err := cmd.app.OpenEngine(ctx)
	if err != nil {
		return nil, nil, err
	}
	var local sessionapi.API = sessionapi.NewLocal(rt.Engine())
	if cmd.app.Config.Experimental.Daemon {
		local = cmd.app.DaemonClient(true)
	}
	fleet := servers.New(cmd.app.Logger, local, cmd.app.Config.Servers, cmd.app.Build.Version)
	handlers := engineops.New(cmd.app.Logger, rt.Engine(), engineops.WithFleet(func() *servers.Fleet { return fleet }))
	return handlers, func() error {
		fleet.Close()
		return rt.Close()
	}, nil
}

func splitToolsets(values []string) []string {
	var names []string
	for _, v := range values {
		for name := range strings.SplitSeq(v, ",") {
			if name = strings.TrimSpace(name); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// newMCPServer mounts a tool for each catalog operation in the named
// toolsets, or in all of them when toolsets is empty. An operation that
// runsInProcess goes to engine; any other is forwarded to the desktop.
func newMCPServer(client *DesktopClient, engine *localEngine, caller string, toolsets []string, version string, log zerolog.Logger) (*mcp.Server, error) {
	selected, err := selectOps(toolsets)
	if err != nil {
		return nil, err
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "hive", Version: version}, nil)
	for _, op := range selected {
		tool, err := forwardedTool(op)
		if err != nil {
			return nil, err
		}
		if runsInProcess(op.Info()) {
			srv.AddTool(tool, engineHandler(engine, caller, op.Info()))
		} else {
			srv.AddTool(tool, forwardHandler(client, caller, op.Info(), log))
		}
	}
	return srv, nil
}

func selectOps(toolsets []string) ([]ops.Op, error) {
	var served []ops.Op
	var valid []string
	for _, op := range catalog.All() {
		info := op.Info()
		served = append(served, op)
		if !slices.Contains(valid, info.Toolset.Name) {
			valid = append(valid, info.Toolset.Name)
		}
	}
	for _, name := range toolsets {
		if !slices.Contains(valid, name) {
			return nil, fmt.Errorf("unknown toolset %q; valid toolsets: %s", name, strings.Join(valid, ", "))
		}
	}
	if len(toolsets) == 0 {
		return served, nil
	}
	return slices.DeleteFunc(served, func(op ops.Op) bool {
		return !slices.Contains(toolsets, op.Info().Toolset.Name)
	}), nil
}

// forwardedTool is the tool the desktop advertises, minus the caller field:
// serve fills it, so the agent must not.
func forwardedTool(op ops.Op) (*mcp.Tool, error) {
	info := op.Info()
	schema, err := op.InputSchema()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", info.Name, err)
	}
	dropCaller(schema)
	return &mcp.Tool{
		Name:        info.ToolName,
		Title:       info.Summary,
		Description: info.Description(),
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: info.Effect == ops.EffectRead},
	}, nil
}

func dropCaller(schema *jsonschema.Schema) {
	delete(schema.Properties, ops.CallerField)
	schema.Required = slices.DeleteFunc(schema.Required, func(name string) bool { return name == ops.CallerField })
}

func forwardHandler(client *DesktopClient, caller string, info ops.Info, log zerolog.Logger) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := forwardedArgs(req.Params.Arguments, info, caller)
		if err != nil {
			return refusal(ops.KindInvalid, err.Error()), nil
		}

		res, err := client.CallWithin(ctx, forwardTimeout(info, args), info.Toolset.MCPPath, info.ToolName, args)
		if err == nil {
			return res, nil
		}
		var unreachable *desktopUnreachableError
		var refused *ops.Error
		switch {
		case errors.As(err, &refused):
			log.Warn().Err(err).Str("tool", info.ToolName).Msg("desktop refused the call")
			return refusal(refused.Kind, refused.Msg), nil
		case errors.As(err, &desktopNotRunningError{}):
			log.Debug().Str("tool", info.ToolName).Msg("desktop is not running")
			return refusal(ops.KindUnavailable, "Hive Desktop is not running"), nil
		case errors.As(err, &unreachable):
			log.Warn().Err(err).Str("tool", info.ToolName).Msg("desktop is unreachable")
			return refusal(ops.KindUnavailable, err.Error()), nil
		}
		log.Warn().Err(err).Str("tool", info.ToolName).Msg("desktop call failed")
		return nil, err
	}
}

func engineHandler(engine *localEngine, caller string, info ops.Info) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := forwardedArgs(req.Params.Arguments, info, caller)
		if err != nil {
			return refusal(ops.KindInvalid, err.Error()), nil
		}
		res, err := engine.Call(ctx, info.ToolName, args)
		if refused, ok := errors.AsType[*ops.Error](err); ok {
			return refusal(refused.Kind, refused.Msg), nil
		}
		return res, err
	}
}

// forwardedArgs sets caller on an operation that declares it and removes it
// from one that does not. The desktop validates against a schema that rejects
// unknown fields, and an agent may still send caller out of habit.
func forwardedArgs(raw json.RawMessage, info ops.Info, caller string) (map[string]any, error) {
	args := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("arguments are not a JSON object: %w", err)
		}
	}
	if info.Caller == ops.CallerRuleNone {
		delete(args, ops.CallerField)
	} else {
		args[ops.CallerField] = caller
	}
	return args, nil
}

// forwardTimeout gives a call that waits, such as message_list with wait, its
// wait on top of the usual timeout. A session-control call clones, runs rules
// or checks a checkout for unsaved work, which takes minutes on a big
// repository; a timeout there makes an agent retry an operation that is still
// running.
func forwardTimeout(info ops.Info, args map[string]any) time.Duration {
	if info.Toolset == sessions.ControlToolset {
		return sessionControlTimeout
	}
	if wait, ok := args["wait"].(float64); ok && wait > 0 {
		return desktopCallTimeout + time.Duration(wait)*time.Second
	}
	return desktopCallTimeout
}

func refusal(kind ops.Kind, msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(kind) + ": " + msg}},
	}
}
