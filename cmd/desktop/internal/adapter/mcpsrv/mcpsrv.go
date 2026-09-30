// Package mcpsrv is the agent-facing MCP adapter over app.App: the desktop's
// capabilities as MCP tools, served over Streamable HTTP on the same loopback
// server that hosts the webhook listener (ADR agent-http-api). It replaces the REST
// control surface that preceded it — an agent discovers what the app can do
// through tools/list rather than an OpenAPI document (ADR mcp-replaces-the-agent-facing-http-api).
//
// Two properties are load-bearing:
//
// The server is stateless, so every request carries its own short-lived
// session. Nothing here holds a client between calls, which is what lets the
// adapter join no lifecycle at all: there are no long-lived SSE streams for
// App.Close to unwind, and the shutdown hazard ADR terminal-transport records for the
// terminal WebSocket does not arise.
//
// The tool set is reads and safe mutations only. Anything that spawns a
// process — terminal and agent-workspace session control — stays on httpapi's
// token-guarded terminal prefix, which is why this surface can keep the
// unauthenticated-behind-loopback posture the REST API had (ADR terminal-transport,
// ADR a-workspace-declares-its-own-authority).
package mcpsrv

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// PathPrefix is where App mounts this handler on the loopback server. It sits
// outside /api/ because that path space belongs to the HTTP adapter, which
// still serves the terminal control planes and the liveness probe.
const PathPrefix = "/mcp"

// serverName identifies this implementation to a client. It is also the id a
// workspace names in its mcps: list, so the two must agree — see
// mcpcatalog's hive-desktop entry.
const serverName = "hive-desktop"

// Options configures the adapter. Version identifies the running build in the
// server-info a client reads on initialize; empty is reported as "dev".
type Options struct {
	Version string
}

// Controller holds what every tool handler needs. Named for httpapi's
// Controller rather than for the SDK's Server, so the two adapters read the
// same way and neither shadows mcp.Server.
type Controller struct {
	core *app.App
	log  zerolog.Logger
	opts Options
}

func New(core *app.App, log zerolog.Logger, opts Options) *Controller {
	return &Controller{core: core, log: log, opts: opts}
}

// Server builds the MCP server with the full tool table registered. It is
// exported so a test can drive the same server over an in-memory transport
// rather than over HTTP — the tools are what is under test, not the framing.
func (ctrl *Controller) Server() *mcp.Server {
	version := ctrl.opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Title:   "Hive Desktop",
		Version: version,
		Description: "Observe and operate the running Hive Desktop app: read its inbox, " +
			"feeds, profiles and action catalog, force a source refresh, dry-run a flow, " +
			"and manage the scheduled chats of its agent workspaces.",
	}, nil)
	ctrl.register(srv)
	return srv
}

// Handler serves that server over Streamable HTTP.
//
// Stateless is what makes this adapter lifecycle-free (see the package doc).
// JSONResponse trades the SSE framing for plain application/json, which costs
// nothing here — no tool streams, and no tool needs a server->client request —
// and leaves the surface reachable with curl for the dev tooling that used to
// read the REST API.
//
// DNS-rebinding protection is the SDK's default and is deliberately left on:
// it rejects a loopback request carrying a non-loopback Host, which is the
// attack a browser could otherwise mount against an unauthenticated local
// server.
func (ctrl *Controller) Handler() http.Handler {
	srv := ctrl.Server()
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
}
