package mcpsrv

import (
	"context"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// OrchestratorPathPrefix must match mcpcatalog's hive-orchestrator RuntimePath.
const OrchestratorPathPrefix = "/mcp/orchestrator"

const orchestratorServerName = "hive-orchestrator"

// OrchestratorController authenticates every tool call, because its tools spawn
// and type into sessions. tools/list stays open: a 401 there sends Claude Code
// looking for an OAuth server.
type OrchestratorController struct {
	core *app.App
	log  zerolog.Logger
	opts Options
}

func NewOrchestrator(core *app.App, log zerolog.Logger, opts Options) *OrchestratorController {
	return &OrchestratorController{core: core, log: log, opts: opts}
}

func (ctrl *OrchestratorController) Server() *mcp.Server {
	version := ctrl.opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    orchestratorServerName,
		Title:   "Hive Orchestrator",
		Version: version,
		Description: "Drive hive sessions from an orchestration workspace: start a session in a repository with a " +
			"prompt, watch its agent, type into it, answer its prompts, and coordinate over the message bus.",
	}, nil)
	ctrl.register(srv)
	return srv
}

func (ctrl *OrchestratorController) Handler() http.Handler {
	srv := ctrl.Server()
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
}

func (ctrl *OrchestratorController) toolError(err error) error { return toolError(ctrl.log, err) }

func (ctrl *OrchestratorController) caller(ctx context.Context, req *mcp.CallToolRequest) (app.OrchestratorCaller, error) {
	var token string
	if req != nil && req.Extra != nil {
		token = bearer(req.Extra.Header.Get("Authorization"))
	}
	caller, err := ctrl.core.Orchestration.Authorize(ctx, token)
	if err != nil {
		return app.OrchestratorCaller{}, ctrl.toolError(err)
	}
	return caller, nil
}

func bearer(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
