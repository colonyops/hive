package mcpsrv

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/hive/engineops"
)

// registerEngine mounts the engine operations whose toolset this server
// serves. They run on the same engine the hive CLI opens, so a task an agent
// claims here is the task `hive task next` hands out.
func (ctrl *Controller) registerEngine(srv *mcp.Server, path string) {
	for _, op := range engineops.New(ctrl.log, ctrl.core.Engine()).Ops() {
		if op.Spec.Info().Toolset.MCPPath == path {
			op.Mount(srv, ctrl.toolError)
		}
	}
}
