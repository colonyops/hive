// Package diagnostics declares the status and diagnostics operations of the
// desktop's app-control MCP server.
package diagnostics

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "diagnostics", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{StatusGet, DiagnosticsGet}
}

var StatusGet = ops.Spec[ops.NoInput]{
	Name: "status.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "Desktop status",
	Guidance: "Report the running build and whether the webhook listener is up, on which host and port. " +
		"Use this first to confirm the app is reachable and to discover the webhook base URL a push should go to.",
}

type DiagnosticsGetIn struct {
	Source    string   `json:"source,omitempty"    jsonschema:"desktop, cli, jobs, or empty for all sources."`
	Since     string   `json:"since,omitempty"     jsonschema:"Inclusive RFC3339 start time."`
	Until     string   `json:"until,omitempty"     jsonschema:"Inclusive RFC3339 end time."`
	Levels    []string `json:"levels,omitempty"    jsonschema:"Any of debug, info, warn, error, and unknown; empty includes all levels."`
	Search    string   `json:"search,omitempty"    jsonschema:"Case-insensitive text to find."`
	Reference string   `json:"reference,omitempty" jsonschema:"Entry ID for surrounding evidence. Job IDs are job-<id>. Overrides time/text filters."`
	Limit     int      `json:"limit,omitempty"     jsonschema:"Maximum entries, default 100, capped at 1000."`
	Detail    string   `json:"detail,omitempty"    jsonschema:"summary (default) omits raw records; full includes them."`
}

var DiagnosticsGet = ops.Spec[DiagnosticsGetIn]{
	Name: "diagnostics.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "Read diagnostics",
	Guidance: "Read bounded Desktop/CLI log tails and job outcomes, with source paths, last-write times, and truncation indicators. " +
		"No processes are started. Use a reference to fetch surrounding evidence, or source jobs and search to find an operation. " +
		"Missing references answer not_found; historical evidence outside the retained tail may be unavailable.",
	Fields: ops.Constraints{"detail": ops.SummaryOrFull},
}
