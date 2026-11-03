package mcpsrv

import (
	"cmp"
	"context"
	"unicode/utf8"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/diagnostics"
)

type diagnosticsResult struct {
	app.DiagnosticsSnapshot
	Detail string `json:"detail"`
}

func (ctrl *Controller) ReadDiagnostics(ctx context.Context, in diagnostics.DiagnosticsGetIn) (diagnosticsResult, error) {
	in.Detail = cmp.Or(in.Detail, ops.DetailSummary)
	if in.Limit == 0 {
		in.Limit = 100
	}
	result, err := ctrl.core.Diagnostics.Read(ctx, app.DiagnosticsQuery{Source: in.Source, Since: in.Since, Until: in.Until, Levels: in.Levels, Search: in.Search, Reference: in.Reference, Limit: in.Limit})
	if in.Detail == ops.DetailSummary {
		for i := range result.Entries {
			result.Entries[i].Raw = ""
			if len(result.Entries[i].Message) > 500 {
				message := result.Entries[i].Message[:500]
				for !utf8.ValidString(message) {
					message = message[:len(message)-1]
				}
				result.Entries[i].Message = message
				result.Entries[i].Truncated = true
			}
		}
	}
	return diagnosticsResult{DiagnosticsSnapshot: result, Detail: in.Detail}, err
}
