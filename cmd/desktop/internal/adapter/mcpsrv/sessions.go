package mcpsrv

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/pkg/logutils"
)

// SessionsPathPrefix must agree with the session-control toolset's MCPPath
// and with mcpcatalog's hive-sessions RuntimePath.
const SessionsPathPrefix = "/mcp/sessions"

// sessionsServerName is also the id a workspace names in its mcps: list.
const sessionsServerName = "hive-sessions"

// SessionsController serves the session operations that start a process
// (session_create, session_start). It shares the chats server's token but is
// its own server and catalogue entry, so granting hive-chats does not also
// grant creating sessions on every repository and server.
type SessionsController struct {
	core  *app.App
	log   zerolog.Logger
	base  zerolog.Logger
	opts  Options
	token string
}

// NewSessions panics on an empty token, as NewChats does.
func NewSessions(core *app.App, log zerolog.Logger, opts Options, token string) *SessionsController {
	if token == "" {
		panic("mcpsrv: the sessions server needs a token")
	}
	return &SessionsController{core: core, log: logutils.Component(log, "mcp.sessions"), base: log, opts: opts, token: token}
}

func (ctrl *SessionsController) Server() *mcp.Server {
	version := ctrl.opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    sessionsServerName,
		Title:   "Hive Sessions",
		Version: version,
		Description: "Create and start hive sessions on this machine or on a configured server. " +
			"A started session runs its agent command in a new terminal.",
	}, nil)
	mountEngine(srv, ctrl.core, ctrl.base, SessionsPathPrefix, ctrl.toolError)
	return srv
}

func (ctrl *SessionsController) Handler() http.Handler {
	srv := ctrl.Server()
	inner := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	return guard(sessionsServerName, ctrl.token, inner)
}

func (ctrl *SessionsController) toolError(err error) error { return toolError(ctrl.log, err) }
