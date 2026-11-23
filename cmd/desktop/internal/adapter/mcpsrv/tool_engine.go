package mcpsrv

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/hive/engineops"
)

// registerEngine mounts the engine operations whose toolset this server
// serves. They run on the same engine the hive CLI opens, so a task an agent
// claims here is the task `hive hc next` hands out.
func (ctrl *Controller) registerEngine(srv *mcp.Server, path string) {
	mountEngine(srv, ctrl.core, ctrl.log, path, ctrl.toolError)
}

func mountEngine(srv *mcp.Server, core *app.App, log zerolog.Logger, path string, mapErr func(error) error) {
	for _, op := range engineops.New(log, core.Engine()).Ops() {
		if op.Spec.Info().Toolset.MCPPath == path {
			op.Mount(srv, mapErr)
		}
	}
}
