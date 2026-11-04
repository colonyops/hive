package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
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
				Usage: "Serve Hive Desktop's tools over stdio",
				Description: `Runs an MCP server on stdin and stdout that forwards each tool call to the
running Hive Desktop, so an agent needs no port. Add it to the agent's config:

  Claude Code: claude mcp add hive-drive -- hive mcp serve --toolset drive,layout
  Codex:       codex mcp add hive-drive -- hive mcp serve --toolset drive,layout

The tools are the same ones "hive dt" calls. --toolset picks which (default:
all). The server finds the desktop through the endpoint file in --data-dir on
every call, so a desktop restart needs no reconnect. While the desktop is not
running, a call answers "unavailable: Hive Desktop is not running".`,
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
	srv, err := newMCPServer(client, caller, splitToolsets(c.StringSlice(toolsetFlag)), cmd.app.Build.Version, log)
	if err != nil {
		return err
	}
	log.Info().Str("caller", caller).Msg("serving MCP on stdio")
	// The protocol owns stdout. Logs go to the log file, so nothing else may
	// print there while this runs.
	return srv.Run(ctx, &mcp.StdioTransport{})
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

// newMCPServer mounts a forwarding tool for each desktop-owned catalog
// operation in the named toolsets, or in all of them when toolsets is empty.
func newMCPServer(client *DesktopClient, caller string, toolsets []string, version string, log zerolog.Logger) (*mcp.Server, error) {
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
		srv.AddTool(tool, forwardHandler(client, caller, op.Info(), log))
	}
	return srv, nil
}

func selectOps(toolsets []string) ([]ops.Op, error) {
	var served []ops.Op
	var valid []string
	for _, op := range catalog.All() {
		info := op.Info()
		if info.Owner != ops.OwnerDesktop {
			continue
		}
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
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return refusal(ops.KindInvalid, fmt.Sprintf("arguments are not a JSON object: %v", err)), nil
			}
		}
		args[ops.CallerField] = caller

		res, err := client.Call(ctx, info.Toolset.MCPPath, info.ToolName, args)
		if err == nil {
			return res, nil
		}
		var unreachable *desktopUnreachableError
		switch {
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

func refusal(kind ops.Kind, msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(kind) + ": " + msg}},
	}
}
