package mcpsrv_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

func TestActionRunToolsReadTheRunAndItsLog(t *testing.T) {
	core, session := testSession(t)
	itemID := seedItem(t, core, "p", "item-1", `{"id":"item-1","title":"Ship it"}`)
	ref := models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "item-1"}

	commands := core.Stores.OutputCommands
	run, created, err := commands.Confirm(t.Context(), "deploy", "item-1", []byte(`{}`), ref, "claim")
	require.NoError(t, err)
	require.True(t, created)
	lines := make([]stores.NewActionRunLogLine, 0, 600)
	lines = append(lines, stores.NewActionRunLogLine{Stream: "system", Text: "$ ./deploy", At: time.UnixMilli(1)})
	for i := range 598 {
		lines = append(lines, stores.NewActionRunLogLine{Stream: "stdout", Text: fmt.Sprintf("step %d", i), At: time.UnixMilli(2)})
	}
	lines = append(lines, stores.NewActionRunLogLine{Stream: "stderr", Text: "boom", At: time.UnixMilli(3)})
	require.NoError(t, core.Stores.ActionRuns.AppendLog(t.Context(), run.ID, 1, lines))
	require.NoError(t, commands.Fail(t.Context(), run.ID, "claim", "exit status 1"))

	var listed struct {
		Runs []app.ActionRunSummary `json:"runs"`
	}
	call(t, session, "list_action_runs", map[string]any{"status": "failed"}, &listed)
	require.Len(t, listed.Runs, 1)
	assert.Equal(t, run.ID, listed.Runs[0].ID)
	assert.Equal(t, "manual", listed.Runs[0].Lane)
	assert.Equal(t, itemID, listed.Runs[0].ItemID)
	assert.Equal(t, "exit status 1", listed.Runs[0].Error)
	assert.NotZero(t, listed.Runs[0].FinishedAt)

	var got struct {
		Run    app.ActionRunSummary `json:"run"`
		Result struct {
			Status string `json:"status"`
			Stderr string `json:"stderr"`
		} `json:"result"`
		Log app.ActionRunLogPage `json:"log"`
	}
	call(t, session, "get_action_run", map[string]any{"id": run.ID}, &got)
	assert.Equal(t, "failed", got.Result.Status)
	require.Len(t, got.Log.Lines, 500, "the default read is the tail")
	assert.Equal(t, "boom", got.Log.Lines[499].Text)
	assert.Equal(t, "stderr", got.Log.Lines[499].Stream)
	assert.False(t, got.Log.More)

	call(t, session, "get_action_run", map[string]any{"id": run.ID, "afterId": 1, "lines": 10}, &got)
	require.Len(t, got.Log.Lines, 10)
	assert.Equal(t, "step 0", got.Log.Lines[0].Text)
	assert.True(t, got.Log.More)
	assert.Equal(t, got.Log.Lines[9].ID, got.Log.NextAfterID)

	assert.Contains(t, callErr(t, session, "get_action_run", map[string]any{"id": 999999}), "not_found")
}
