package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

const defaultActionRunTail = 500

type listActionRunsInput struct {
	Before   int64  `json:"before,omitempty"   jsonschema:"Return runs with an id below this one, to page back; omit for the newest."`
	Limit    int    `json:"limit,omitempty"    jsonschema:"Maximum runs to read; defaults to 100, at most 500."`
	Status   string `json:"status,omitempty"   jsonschema:"Keep only runs in this status: pending, running, done, failed or cancelled."`
	ActionID string `json:"actionId,omitempty" jsonschema:"Keep only runs of this action id."`
}

type actionRunsResult struct {
	Runs []app.ActionRunSummary `json:"runs"`
}

// ListActionRuns filters after the page is read, so a filtered answer can
// hold fewer runs than limit while older matches exist; page with before.
func (ctrl *Controller) ListActionRuns(ctx context.Context, _ *mcp.CallToolRequest, in listActionRunsInput) (*mcp.CallToolResult, actionRunsResult, error) {
	runs, err := ctrl.core.ActionRuns.List(ctx, in.Before, in.Limit)
	if err != nil {
		return nil, actionRunsResult{}, ctrl.toolError(err)
	}
	kept := make([]app.ActionRunSummary, 0, len(runs))
	for _, run := range runs {
		if in.Status != "" && run.Status != in.Status {
			continue
		}
		if in.ActionID != "" && run.ActionID != in.ActionID {
			continue
		}
		kept = append(kept, run)
	}
	return nil, actionRunsResult{Runs: kept}, nil
}

type getActionRunInput struct {
	ID      int64 `json:"id"                jsonschema:"The run id, from list_action_runs."`
	AfterID int64 `json:"afterId,omitempty" jsonschema:"Read log lines after this line id, oldest first. Omit to read the tail instead."`
	Lines   int   `json:"lines,omitempty"   jsonschema:"Maximum log lines to return; defaults to 500, at most 10000."`
}

type actionRunResult struct {
	Run    app.ActionRunSummary   `json:"run"`
	Result dispatch.ActionRunView `json:"result"`
	Log    app.ActionRunLogPage   `json:"log"`
}

func (ctrl *Controller) GetActionRun(ctx context.Context, _ *mcp.CallToolRequest, in getActionRunInput) (*mcp.CallToolResult, actionRunResult, error) {
	if in.ID <= 0 {
		return nil, actionRunResult{}, ctrl.toolError(app.Errorf(app.KindInvalid, "id is required"))
	}
	run, err := ctrl.core.ActionRuns.Get(ctx, in.ID)
	if err != nil {
		return nil, actionRunResult{}, ctrl.toolError(err)
	}
	result, err := ctrl.core.Inbox.ActionRun(ctx, in.ID)
	if err != nil {
		return nil, actionRunResult{}, ctrl.toolError(err)
	}
	lines := in.Lines
	if lines <= 0 {
		lines = defaultActionRunTail
	}
	var log app.ActionRunLogPage
	if in.AfterID > 0 {
		log, err = ctrl.core.ActionRuns.Log(ctx, in.ID, in.AfterID, lines)
	} else {
		log, err = ctrl.core.ActionRuns.Tail(ctx, in.ID, lines)
	}
	if err != nil {
		return nil, actionRunResult{}, ctrl.toolError(err)
	}
	return nil, actionRunResult{Run: run, Result: result, Log: log}, nil
}
