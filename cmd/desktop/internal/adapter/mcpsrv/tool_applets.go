package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// appletsResult closes the loop on an agent's write: a file that failed to parse
// reports its error on its entry, where the applet's own UI would show it.
type appletsResult struct {
	Dir     string       `json:"dir"`
	Applets []app.Applet `json:"applets"`
}

func (ctrl *Controller) ListApplets(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	return nil, appletsResult{Dir: ctrl.core.Applets.Dir(), Applets: ctrl.core.Applets.List(ctx)}, nil
}
