package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type refreshResult struct {
	Sources  int `json:"sources"  jsonschema:"Number of sources that ran this tick."`
	Appended int `json:"appended" jsonschema:"Number of inbox items appended across all sources."`
	Failed   int `json:"failed"   jsonschema:"Number of sources that failed this tick."`
}

func (ctrl *Controller) RefreshSources(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, refreshResult, error) {
	sum, err := ctrl.core.RefreshSources(ctx)
	if err != nil {
		return nil, refreshResult{}, ctrl.toolError(err)
	}
	return nil, refreshResult{Sources: sum.Sources, Appended: sum.Appended, Failed: sum.Failed}, nil
}
