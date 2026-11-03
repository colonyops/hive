package mcpsrv

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/ops/actions"
)

const defaultActionRunTail = 500

type actionRunsResult struct {
	Runs []app.ActionRunSummary `json:"runs"`
}

// ListActionRuns filters after the page is read, so a filtered answer can
// hold fewer runs than limit while older matches exist; page with before.
func (ctrl *Controller) ListActionRuns(ctx context.Context, in actions.ActionRunListIn) (actionRunsResult, error) {
	runs, err := ctrl.core.ActionRuns.List(ctx, in.Before, in.Limit)
	if err != nil {
		return actionRunsResult{}, err
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
	return actionRunsResult{Runs: kept}, nil
}

type actionRunResult struct {
	Run    app.ActionRunSummary   `json:"run"`
	Result dispatch.ActionRunView `json:"result"`
	Log    app.ActionRunLogPage   `json:"log"`
}

func (ctrl *Controller) GetActionRun(ctx context.Context, in actions.ActionRunGetIn) (actionRunResult, error) {
	if in.ID <= 0 {
		return actionRunResult{}, app.Errorf(app.KindInvalid, "id is required")
	}
	run, err := ctrl.core.ActionRuns.Get(ctx, in.ID)
	if err != nil {
		return actionRunResult{}, err
	}
	result, err := ctrl.core.Inbox.ActionRun(ctx, in.ID)
	if err != nil {
		return actionRunResult{}, err
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
		return actionRunResult{}, err
	}
	return actionRunResult{Run: run, Result: result, Log: log}, nil
}
