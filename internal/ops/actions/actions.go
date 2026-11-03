// Package actions declares the action catalog, action run and applet
// operations of the desktop's app-control MCP server.
package actions

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "actions", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{ActionList, ActionRunList, ActionRunGet, AppletList}
}

var ActionList = ops.Spec[ops.NoInput]{
	Name: "action.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List the action catalog",
	Guidance: "List the action catalog with the actions.yml it was loaded from and whether that file currently parses. " +
		"An invalid edit leaves the previous catalog in effect and reports its error here, so this is how to confirm an edit actually loaded.",
}

var AppletList = ops.Spec[ops.NoInput]{
	Name: "applet.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List the applets",
	Guidance: "List the applets with the directory they are written under and, for each, the error in its applet.yaml if it does not load. " +
		"An applet that fails to load keeps its last good file, so this is how to confirm a write actually took.",
}

var RunStatuses = []string{"pending", "running", "done", "failed", "cancelled"}

type ActionRunListIn struct {
	Before   int64  `json:"before,omitempty"   jsonschema:"Return runs with an id below this one, to page back; omit for the newest."`
	Limit    int    `json:"limit,omitempty"    jsonschema:"Maximum runs to read; defaults to 100, at most 500."`
	Status   string `json:"status,omitempty"   jsonschema:"Keep only runs in this status: pending, running, done, failed or cancelled."`
	ActionID string `json:"actionId,omitempty" jsonschema:"Keep only runs of this action id."`
}

var ActionRunList = ops.Spec[ActionRunListIn]{
	Name: "action_run.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List action runs",
	Guidance: "List action runs newest first: every durable execution of an actions.yml action, whether run from an item's detail pane or by a flow action node, " +
		"with its status, attempts, lane (manual or automatic), timestamps, last error and the inbox item it ran against. " +
		"status and actionId filter the page after it is read, so page back with before when a filtered page comes back short. " +
		"Notify and launch nodes are not actions.yml actions and are not listed. Actions run from a terminal session or window are not durable and never appear here. " +
		"Use action_run_get to read what a run printed.",
	Fields: ops.Constraints{"status": {Enum: RunStatuses}},
}

type ActionRunGetIn struct {
	ID      int64 `json:"id"                jsonschema:"The run id, from action_run_list."`
	AfterID int64 `json:"afterId,omitempty" jsonschema:"Read log lines after this line id, oldest first. Omit to read the tail instead."`
	Lines   int   `json:"lines,omitempty"   jsonschema:"Maximum log lines to return; defaults to 500, at most 10000."`
}

var ActionRunGet = ops.Spec[ActionRunGetIn]{
	Name: "action_run.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "Read an action run and its log",
	Guidance: "Return one action run, its result, and its log: the stdout and stderr of any process it ran, interleaved in order, " +
		"plus system lines describing each step (the command line, a session created, a message published, exit codes, retries, cancellation). " +
		"Every line carries its attempt number, so automatic retries read as separate attempts. " +
		"By default it returns the last 500 lines, which is where a failure explains itself; pass afterId to read forward from a line, " +
		"and follow nextAfterId while more is true to read the rest. A running run's log grows, so read again with the last nextAfterId to follow it. " +
		"Each attempt keeps up to 8 MiB of output, and a run's log is kept for as long as the run is (the newest 100 finished runs by default, " +
		"set by retention.action_runs in settings.yaml).",
	Args: []string{"id"},
}
