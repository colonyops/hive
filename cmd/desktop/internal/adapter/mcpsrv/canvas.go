package mcpsrv

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// CanvasPathPrefix is where main mounts the canvas server on the loopback
// server, beside PathPrefix. It must agree with mcpcatalog's hive-canvas
// RuntimePath — TestCanvasCatalogueAgreement pins it.
const CanvasPathPrefix = "/mcp/canvas"

// canvasServerName identifies the canvas server to a client. Like serverName
// it is also the id a workspace names in its mcps: list, so the two must
// agree — see mcpcatalog's hive-canvas entry.
const canvasServerName = "hive-canvas"

// CanvasController is the canvas server's half of what Controller is for the
// app-control surface: a separate server with its own small tool table, so a
// workspace can enable the canvas without the app-driving tools
// (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry).
// Same stateless posture, same loopback threat model.
type CanvasController struct {
	core *app.App
	log  zerolog.Logger
	opts Options
}

func NewCanvas(core *app.App, log zerolog.Logger, opts Options) *CanvasController {
	return &CanvasController{core: core, log: log, opts: opts}
}

// Server builds the canvas MCP server with its tool table registered.
// Exported for the same reason Controller.Server is: tests drive it over an
// in-memory transport.
func (ctrl *CanvasController) Server() *mcp.Server {
	version := ctrl.opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    canvasServerName,
		Title:   "Hive Canvas",
		Version: version,
		Description: "The workspace's canvases: named surfaces of markdown, html and link blocks shown to the user " +
			"in a pane beside the conversation. Put blocks, revise them in place, and read back what is showing.",
	}, nil)
	ctrl.register(srv)
	return srv
}

// Handler serves the canvas server over Streamable HTTP, with the exact
// framing choices mcpsrv.go documents for the app-control server: stateless,
// plain JSON, DNS-rebinding protection on.
func (ctrl *CanvasController) Handler() http.Handler {
	srv := ctrl.Server()
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
}

func (ctrl *CanvasController) toolError(err error) error { return toolError(ctrl.log, err) }
