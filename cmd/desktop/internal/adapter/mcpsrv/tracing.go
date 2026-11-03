package mcpsrv

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/ops/mcpops"
)

// addTool registers a tool that has no catalog Spec yet, with the same span
// and duration metric a mounted Spec gets.
func addTool[In, Out any](srv *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(srv, tool, mcpops.Instrument(tool.Name, handler))
}
