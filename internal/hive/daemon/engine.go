package daemon

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
)

// servedRemotely reports whether the daemon serves op at EnginePath. The
// session operations have SessionService, which the fleet already routes.
func servedRemotely(info ops.Info) bool {
	return info.Owner == ops.OwnerEngine && info.Toolset != sessions.Toolset && info.Toolset != sessions.ControlToolset
}

// engineHandler serves the engine operations over MCP with the handlers the
// desktop and hive mcp serve mount, so a remote call is checked and answered
// as a local one. It has no fleet: a daemon acts on its own machine only.
// Stateless, because a client may reach it over a new ssh connection on any
// call.
func engineHandler(logger zerolog.Logger, engine *hive.Engine, version string) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "hive-daemon", Version: version}, nil)
	internal := func(err error) error {
		logger.Error().Err(err).Msg("engine tool failed")
		return ops.Errorf(ops.KindInternal, "hive could not complete the request")
	}
	for _, op := range engineops.New(logger, engine).Ops() {
		if servedRemotely(op.Spec.Info()) {
			op.Mount(srv, internal)
		}
	}
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}
