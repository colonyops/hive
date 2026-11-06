package mcpsrv

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/pkg/logutils"
)

// DrivePathPrefix is where main mounts the drive server on the loopback
// server, beside PathPrefix. It must agree with mcpcatalog's hive-drive
// RuntimePath; TestDriveCatalogueAgreement pins it.
const DrivePathPrefix = "/mcp/drive"

// driveServerName identifies the drive server to a client. Like serverName
// it is also the id a workspace names in its mcps: list, so the two must
// agree; see mcpcatalog's hive-drive entry.
const driveServerName = "hive-drive"

// DriveController is the drive server's half of what Controller is for the
// app-control surface: a separate server with its own small tool table, so a
// workspace can enable documents without the app-driving tools
// (ADR hive-drive-has-no-canvas-document-kind).
// Same stateless posture, same loopback threat model.
type DriveController struct {
	core *app.App
	log  zerolog.Logger
	opts Options
}

func NewDrive(core *app.App, log zerolog.Logger, opts Options) *DriveController {
	return &DriveController{core: core, log: logutils.Component(log, "mcp.drive"), opts: opts}
}

// Server builds the drive MCP server with its tool table registered.
// Exported for the same reason Controller.Server is: tests drive it over an
// in-memory transport.
func (ctrl *DriveController) Server() *mcp.Server {
	version := ctrl.opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    driveServerName,
		Title:   "Hive Drive",
		Version: version,
		Description: "Hive Drive: the folder of markdown and html documents behind the Drive tab of a chat or Code session. Write and " +
			"organize documents, and read back what the user sees in the Drive tab.",
	}, nil)
	ctrl.register(srv)
	return srv
}

// Handler serves the drive server over Streamable HTTP, with the exact
// framing choices mcpsrv.go documents for the app-control server: stateless,
// plain JSON, DNS-rebinding protection on.
func (ctrl *DriveController) Handler() http.Handler {
	srv := ctrl.Server()
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
}

// register declares the drive tool table, under the same rules as
// Controller.register: metadata only, thin handlers, descriptions written for
// a model.
func (ctrl *DriveController) register(srv *mcp.Server) {
	ctrl.registerDocuments(srv)
	ctrl.registerTabs(srv)
	ctrl.registerReviews(srv)
}

func (ctrl *DriveController) toolError(err error) error { return toolError(ctrl.log, err) }
