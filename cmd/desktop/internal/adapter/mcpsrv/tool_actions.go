package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
)

// actionsResult is the actions catalog as the running app holds it. An
// actions.yml that fails to parse leaves the previous catalog in effect, so the
// action list alone cannot tell an editor whether its write was accepted: valid
// and error are what close that loop. Path names the file to fix when it did
// not.
type actionsResult struct {
	Path    string                   `json:"path"`
	Valid   bool                     `json:"valid"`
	Error   string                   `json:"error"`
	Actions []actions.EditableAction `json:"actions"`
}

func (ctrl *Controller) ListActions(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	catalog := ctrl.core.Actions.List(ctx)
	return nil, actionsResult{
		Path:    ctrl.core.RuntimePaths().ActionsPath,
		Valid:   catalog.Error == "",
		Error:   catalog.Error,
		Actions: catalog.Actions,
	}, nil
}
