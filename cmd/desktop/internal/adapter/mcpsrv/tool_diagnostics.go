package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

type diagnosticsInput struct {
	Source    string `json:"source,omitempty"    jsonschema:"desktop, cli, jobs, or empty for all sources."`
	Since     string `json:"since,omitempty"     jsonschema:"Inclusive RFC3339 start time."`
	Until     string `json:"until,omitempty"     jsonschema:"Inclusive RFC3339 end time."`
	Level     string `json:"level,omitempty"     jsonschema:"debug, info, warn, error, unknown, or empty."`
	Search    string `json:"search,omitempty"    jsonschema:"Case-insensitive text to find."`
	Reference string `json:"reference,omitempty" jsonschema:"Entry ID for surrounding evidence. Job IDs are job-<id>. Overrides time/text filters."`
	Limit     int    `json:"limit,omitempty"     jsonschema:"Maximum entries, default 100, capped at 1000."`
	Detail    string `json:"detail,omitempty"    jsonschema:"summary (default) omits raw records; full includes them."`
}

type diagnosticsResult struct {
	app.DiagnosticsSnapshot
	Detail string `json:"detail"`
}

func (ctrl *Controller) ReadDiagnostics(ctx context.Context, _ *mcp.CallToolRequest, in diagnosticsInput) (*mcp.CallToolResult, diagnosticsResult, error) {
	if in.Detail == "" {
		in.Detail = detailSummary
	}
	if in.Detail != detailSummary && in.Detail != detailFull {
		return nil, diagnosticsResult{}, ctrl.toolError(app.Errorf(app.KindInvalid, "detail must be summary or full"))
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	result, err := ctrl.core.Diagnostics.Read(ctx, app.DiagnosticsQuery{Source: in.Source, Since: in.Since, Until: in.Until, Level: in.Level, Search: in.Search, Reference: in.Reference, Limit: in.Limit})
	if in.Detail == detailSummary {
		for i := range result.Entries {
			result.Entries[i].Raw = ""
			if len(result.Entries[i].Message) > 500 {
				result.Entries[i].Message = result.Entries[i].Message[:500]
				result.Entries[i].Truncated = true
			}
		}
	}
	return nil, diagnosticsResult{DiagnosticsSnapshot: result, Detail: in.Detail}, ctrl.toolError(err)
}
