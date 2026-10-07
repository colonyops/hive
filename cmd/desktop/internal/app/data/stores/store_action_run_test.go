package stores

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func confirmTestRun(t *testing.T, st *Stores, actionID string) OutputCommand {
	t.Helper()
	row, created, err := st.OutputCommands.Confirm(t.Context(), actionID, "key", []byte(`{}`), models.ItemRef{}, "claim-"+actionID)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, st.ActionRuns.AppendLog(t.Context(), row.ID, 1, []NewActionRunLogLine{{Stream: "stdout", Text: "hello " + actionID}}))
	return row
}

func TestActionRunStore_LogTailAndPaging(t *testing.T) {
	st, _ := openTestStores(t)
	row := confirmTestRun(t, st, "a")
	lines := make([]NewActionRunLogLine, 0, 9)
	for i := range 9 {
		lines = append(lines, NewActionRunLogLine{Stream: "stdout", Text: fmt.Sprint(i)})
	}
	require.NoError(t, st.ActionRuns.AppendLog(t.Context(), row.ID, 2, lines))

	after, err := st.ActionRuns.TailCursor(t.Context(), row.ID, 3)
	require.NoError(t, err)
	tail, err := st.ActionRuns.ListLog(t.Context(), row.ID, after, 10)
	require.NoError(t, err)
	require.Len(t, tail, 3)
	assert.Equal(t, []string{"6", "7", "8"}, []string{tail[0].Text, tail[1].Text, tail[2].Text})
	assert.Equal(t, int64(2), tail[0].Attempt)

	after, err = st.ActionRuns.TailCursor(t.Context(), row.ID, 100)
	require.NoError(t, err)
	assert.Zero(t, after, "a log shorter than the tail reads from the start")
}

func TestActionRunStore_ListJoinsTheFinishedCommand(t *testing.T) {
	st, _ := openTestStores(t)
	row := confirmTestRun(t, st, "deploy")
	require.NoError(t, st.OutputCommands.Complete(t.Context(), row.ID, row.ClaimToken, ""))

	runs, err := st.ActionRuns.List(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "done", runs[0].Status)
	assert.Equal(t, "manual", runs[0].DispatchLane)
	assert.NotZero(t, runs[0].FinishedAt)
	assert.NotZero(t, runs[0].ClaimedAt)
}

func TestActionRunLogGoesWithItsRun(t *testing.T) {
	st, db := openTestStores(t)
	active := confirmTestRun(t, st, "active")
	old := confirmTestRun(t, st, "old")
	require.NoError(t, st.OutputCommands.Complete(t.Context(), old.ID, old.ClaimToken, ""))
	newest := confirmTestRun(t, st, "newest")
	require.NoError(t, st.OutputCommands.Complete(t.Context(), newest.ID, newest.ClaimToken, ""))

	policy := queries.DefaultRetentionPolicy()
	policy.ActionRunLimit = 1
	require.NoError(t, db.Prune(t.Context(), policy))

	for id, want := range map[int64]int{active.ID: 1, old.ID: 0, newest.ID: 1} {
		lines, err := st.ActionRuns.ListLog(t.Context(), id, 0, 10)
		require.NoError(t, err)
		assert.Len(t, lines, want, "command %d", id)
	}
}
