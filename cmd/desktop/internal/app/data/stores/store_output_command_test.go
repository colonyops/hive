package stores

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func claimNextTestCommand(t *testing.T, st *Stores) OutputCommand {
	t.Helper()
	row, found, err := st.OutputCommands.ClaimNextAutomatic(t.Context(), 0, "test-claim")
	require.NoError(t, err)
	require.True(t, found)
	return row
}

func enqueueTestCommand(t *testing.T, st *Stores, actionID, key string) {
	t.Helper()
	require.NoError(t, st.EventLog.Commit(t.Context(), models.CommitBatch{
		Consumer:   "flow-" + actionID + "-" + key,
		UpToOffset: 1,
		Outputs: []models.Output{
			{
				Sink:          models.Sink{Kind: models.SinkKindAction, TargetID: actionID},
				OccurrenceKey: key,
				Payload:       []byte(`{"v":1}`),
			},
		},
	}))
}

func TestCommit_RecordsTheItemAnActionCommandCameFrom(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	routed := models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "acct", ExternalID: "acme/repo#1"}

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "p", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindAction, TargetID: "review-pr"}, Key: "acme/repo#1",
			OccurrenceKey: "oc-1", Payload: []byte(`{"v":1}`), SourceKind: "github", SourceScope: "acct",
		}},
	}))

	rows, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "oc-1", rows[0].Key)
	assert.Equal(t, routed, rows[0].ItemRef(), "the enqueued command carries the item the flow fired it from")
}

func TestConfirm_KeepsTheRoutedOriginAndFillsAMissingOne(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	routed := models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "acct", ExternalID: "acme/repo#1"}

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "p", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindAction, TargetID: "review-pr"}, Key: "acme/repo#1",
			OccurrenceKey: "oc-1", Payload: []byte(`{"v":1}`), SourceKind: "github", SourceScope: "acct",
		}},
	}))
	other := models.ItemRef{ProfileID: "q", SourceKind: "webhook", ExternalID: "other"}
	claimed, _, err := st.OutputCommands.Confirm(ctx, "review-pr", "oc-1", []byte(`{"v":1}`), other, "claim-1")
	require.NoError(t, err)
	assert.Equal(t, routed, claimed.ItemRef(), "a confirm never overwrites the origin the flow recorded")

	// A command with no stored origin takes the caller's complete ItemRef.
	_, err = db.Conn().ExecContext(ctx,
		`INSERT INTO output_command (action_id, key, payload, status, created_at) VALUES ('shell-it', 'oc-2', CAST('{}' AS BLOB), 'pending', 1)`)
	require.NoError(t, err)
	filled, _, err := st.OutputCommands.Confirm(ctx, "shell-it", "oc-2", []byte(`{"v":1}`), routed, "claim-2")
	require.NoError(t, err)
	assert.Equal(t, routed, filled.ItemRef())
}

func TestRecoverInterruptedOutputCommands_JoinsTransaction(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	command, created, err := st.OutputCommands.Confirm(ctx, "review", "item-1", []byte(`{}`), models.ItemRef{}, "claim")
	require.NoError(t, err)
	require.True(t, created)
	job, err := st.Jobs.Insert(ctx, JobCreate{Status: "queued", Label: "Review"})
	require.NoError(t, err)
	_, err = st.Jobs.SetRunning(ctx, job.ID, "Running", command.ID)
	require.NoError(t, err)

	require.NoError(t, db.RecoverInterruptedOutputCommands(ctx))

	command, err = st.OutputCommands.Get(ctx, command.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", command.Status)
	jobs, err := st.Jobs.List(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "failed", jobs[0].Status)
	rows, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestListRunnableOutputCommands_ReturnsOldestIDFirst(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	enqueueTestCommand(t, st, "action-a", "k1")
	enqueueTestCommand(t, st, "action-a", "k2")

	rows, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "k1", rows[0].Key)
	assert.Equal(t, "k2", rows[1].Key)
	assert.Equal(t, "pending", rows[0].Status)
	assert.Equal(t, int64(0), rows[0].Attempts)
	assert.Empty(t, rows[0].LastError)
}

func TestListRunnableOutputCommands_RespectsLimit(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	enqueueTestCommand(t, st, "action-a", "k1")
	enqueueTestCommand(t, st, "action-a", "k2")

	rows, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "k1", rows[0].Key)
}

// A guarded conflict returns the existing command with created=false, so an
// already-running or completed action cannot fire again.
func TestConfirmOutputCommandDeduplicatesExistingCommand(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	enqueueTestCommand(t, st, "action-a", "k1")
	row, created, err := st.OutputCommands.Confirm(ctx, "action-a", "k1", []byte(`{"v":2}`), models.ItemRef{}, "claim")
	require.NoError(t, err)
	assert.True(t, created, "confirmation atomically claims the pending command")
	assert.Equal(t, "running", row.Status)
	assert.JSONEq(t, `{"v":1}`, string(row.Payload), "the queued payload remains authoritative")
	existing, created, err := st.OutputCommands.Confirm(ctx, "action-a", "k1", []byte(`{"v":3}`), models.ItemRef{}, "other-claim")
	require.NoError(t, err)
	assert.False(t, created, "a running command cannot be claimed twice")
	assert.Equal(t, row.ID, existing.ID, "the existing command is returned for confirmation UX")
	require.NoError(t, st.OutputCommands.Complete(ctx, row.ID, row.ClaimToken, "", "", ""))

	rerun, err := st.OutputCommands.Rerun(ctx, "action-a", "k1", []byte(`{"v":4}`), models.ItemRef{}, "rerun-claim")
	require.NoError(t, err)
	assert.NotEqual(t, row.ID, rerun.ID)
	assert.True(t, rerun.IsRerun)
	assert.JSONEq(t, `{"v":4}`, string(rerun.Payload))
}

// No completed prior run is a not-found result: IsNotFound must stay true and
// the error must continue to unwrap to sql.ErrNoRows.
func TestRerunOutputCommandRequiresPriorRun(t *testing.T) {
	st, _ := openTestStores(t)

	_, err := st.OutputCommands.Rerun(t.Context(), "action-a", "missing", []byte(`{}`), models.ItemRef{}, "claim")
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
}

func TestRerunOutputCommandRejectsActivePriorRun(t *testing.T) {
	st, _ := openTestStores(t)
	enqueueTestCommand(t, st, "action-a", "active")

	_, err := st.OutputCommands.Rerun(t.Context(), "action-a", "active", []byte(`{}`), models.ItemRef{}, "claim")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOutputCommandActive)
}

func TestMarkOutputCommandDone_ExcludesFromRunnable(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	enqueueTestCommand(t, st, "action-a", "k1")
	claimed := claimNextTestCommand(t, st)
	require.NoError(t, st.OutputCommands.Complete(ctx, claimed.ID, claimed.ClaimToken, "", "", ""))

	rows, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestRequeueOutputCommandStaysRunnableAndCountsClaims(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	enqueueTestCommand(t, st, "action-a", "k1")
	claimed := claimNextTestCommand(t, st)
	require.NoError(t, st.OutputCommands.Requeue(ctx, claimed.ID, claimed.ClaimToken, "boom", "", "", 0))

	rows, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "retried command stays runnable")
	assert.Equal(t, int64(1), rows[0].Attempts)
	assert.Equal(t, "boom", rows[0].LastError)

	claimed = claimNextTestCommand(t, st)
	require.NoError(t, st.OutputCommands.Requeue(ctx, claimed.ID, claimed.ClaimToken, "boom again", "", "", 0))
	rows, err = st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), rows[0].Attempts)
}

func TestMarkOutputCommandDoneClearsPreviousFailure(t *testing.T) {
	st, _ := openTestStores(t)
	enqueueTestCommand(t, st, "action-a", "k1")
	claimed := claimNextTestCommand(t, st)
	require.NoError(t, st.OutputCommands.Requeue(t.Context(), claimed.ID, claimed.ClaimToken, "first failure", "old stdout", "old stderr", 0))
	claimed = claimNextTestCommand(t, st)
	require.NoError(t, st.OutputCommands.Complete(t.Context(), claimed.ID, claimed.ClaimToken, `{"message":{"topic":"agent.inbox"}}`, "new stdout", ""))

	row, err := st.OutputCommands.Get(t.Context(), claimed.ID)
	require.NoError(t, err)
	assert.Equal(t, "done", row.Status)
	assert.Empty(t, row.LastError, "successful retry must not retain stale failure")
	assert.JSONEq(t, `{"message":{"topic":"agent.inbox"}}`, row.ResultJSON)
	assert.Equal(t, "new stdout", row.Stdout)
	assert.Empty(t, row.Stderr)
}

func TestOutputCommandPersistenceBoundsStreams(t *testing.T) {
	st, _ := openTestStores(t)
	enqueueTestCommand(t, st, "action-a", "k1")
	claimed := claimNextTestCommand(t, st)
	noisy := strings.Repeat("x", maxOutputCommandStreamBytes+1)
	require.NoError(t, st.OutputCommands.Fail(t.Context(), claimed.ID, claimed.ClaimToken, "failed", noisy, noisy))

	row, err := st.OutputCommands.Get(t.Context(), claimed.ID)
	require.NoError(t, err)
	assert.Len(t, row.Stdout, maxOutputCommandStreamBytes)
	assert.Len(t, row.Stderr, maxOutputCommandStreamBytes)
	assert.True(t, strings.HasSuffix(row.Stdout, outputCommandTruncatedMarker))
	assert.True(t, strings.HasSuffix(row.Stderr, outputCommandTruncatedMarker))
}

func TestExecutionResultAndLogsPersistAcrossReopenBeforeDone(t *testing.T) {
	dir := t.TempDir()
	db, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	st := New(db, Options{})
	enqueueTestCommand(t, st, "action-a", "k1")
	claimed := claimNextTestCommand(t, st)
	require.NoError(t, st.OutputCommands.Complete(t.Context(), claimed.ID, claimed.ClaimToken, `{"message":{"topic":"agent.inbox"}}`, "stdout", "stderr"))
	require.NoError(t, db.Close())

	db, err = queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	st = New(db, Options{})
	row, err := st.OutputCommands.Get(t.Context(), claimed.ID)
	require.NoError(t, err)
	assert.Equal(t, "done", row.Status)
	assert.JSONEq(t, `{"message":{"topic":"agent.inbox"}}`, row.ResultJSON)
	assert.Equal(t, "stdout", row.Stdout)
	assert.Equal(t, "stderr", row.Stderr)
}

func TestMarkOutputCommandFailed_ExcludesFromRunnable(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	enqueueTestCommand(t, st, "action-a", "k1")
	claimed := claimNextTestCommand(t, st)
	id := claimed.ID

	require.NoError(t, st.OutputCommands.Fail(ctx, id, claimed.ClaimToken, "gave up", "", ""))

	rows, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)

	var status string
	var lastErr string
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT status, last_error FROM output_command WHERE id = ?`, id,
	).Scan(&status, &lastErr))
	assert.Equal(t, "failed", status)
	assert.Equal(t, "gave up", lastErr)
}

func TestOutputCommandClaimsAreAtomic(t *testing.T) {
	st, _ := openTestStores(t)
	enqueueTestCommand(t, st, "action-a", "k1")
	enqueueTestCommand(t, st, "action-a", "k2")

	type claimResult struct {
		id    int64
		found bool
		err   error
	}
	var wg sync.WaitGroup
	results := make(chan claimResult, 2)
	for _, token := range []string{"claim-a", "claim-b"} {
		wg.Go(func() {
			row, found, err := st.OutputCommands.ClaimNextAutomatic(t.Context(), 0, token)
			results <- claimResult{id: row.ID, found: found, err: err}
		})
	}
	wg.Wait()
	close(results)
	claimed := make(map[int64]struct{})
	for result := range results {
		require.NoError(t, result.err)
		require.True(t, result.found)
		claimed[result.id] = struct{}{}
	}
	assert.Len(t, claimed, 2)
}

func TestOutputCommandTerminalTransitionRejectsStaleClaim(t *testing.T) {
	st, _ := openTestStores(t)
	enqueueTestCommand(t, st, "action-a", "k1")
	claimed := claimNextTestCommand(t, st)
	require.NoError(t, st.OutputCommands.Complete(t.Context(), claimed.ID, claimed.ClaimToken, "", "", ""))
	require.ErrorContains(t,
		st.OutputCommands.Complete(t.Context(), claimed.ID, claimed.ClaimToken, "", "", ""),
		"stale claim",
	)
}

func TestOutputCommandStore_CountNonterminalForAction(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	enqueueTestCommand(t, st, "action-a", "k1")
	enqueueTestCommand(t, st, "action-a", "k2")
	enqueueTestCommand(t, st, "action-b", "k3")

	count, err := st.OutputCommands.CountNonterminalForAction(ctx, "action-a")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	for range 3 {
		claimed := claimNextTestCommand(t, st)
		require.NoError(t, st.OutputCommands.Complete(ctx, claimed.ID, claimed.ClaimToken, "", "", ""))
	}

	count, err = st.OutputCommands.CountNonterminalForAction(ctx, "action-a")
	require.NoError(t, err)
	assert.Zero(t, count, "a terminal command no longer counts")
}
