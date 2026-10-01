package stores

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func observation(payload string) models.Observation {
	return models.Observation{ExternalID: "acme/repo#1", Title: "one", URL: "https://example.test/1", SourceKind: "test", ObservedAt: 100, Payload: []byte(payload)}
}

func TestIngestObservation_DuplicatePayloadWritesNothing(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	current := observation(`{"v":1}`)
	first, err := st.InboxItems.IngestObservation(ctx, activityClassifier("one"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	require.True(t, first.Wrote)
	second, err := st.InboxItems.IngestObservation(ctx, activityClassifier("one"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	assert.False(t, second.Wrote)
	var revision, events int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT revision FROM inbox_item`).Scan(&revision))
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT count(*) FROM inbox_event`).Scan(&events))
	assert.Equal(t, 1, revision)
	assert.Equal(t, 1, events)
}

// A changed item from before hay-kot/hive-desktop#63 (empty source_scope) must be rewritten onto the
// account scope in place, not forked into a scoped duplicate beside the
// original — which would strand the row that carries the user's triage
// decisions. See issue hay-kot/hive-desktop#95.
func TestIngestObservation_HealsChangedLegacyEmptyScopeItemInPlace(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	legacy, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: "p", SourceKind: "github", SourceScope: "", ExternalID: "acme/repo#1",
		Payload: []byte(`{"v":1}`), Lifecycle: "active", Unread: 1,
	})
	require.NoError(t, err)

	current := models.Observation{ExternalID: "acme/repo#1", Title: "one", SourceKind: "github", SourceScope: "acct", ObservedAt: 100, Payload: []byte(`{"v":2}`)}
	result, err := st.InboxItems.IngestObservation(ctx, activityClassifier("one"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	require.True(t, result.Wrote)
	assert.Equal(t, legacy.ID, result.ItemID, "the changed item heals the existing row rather than inserting a new one")

	var rows int
	var scope string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_item WHERE external_id = ?`, "acme/repo#1").Scan(&rows))
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT source_scope FROM inbox_item WHERE id = ?`, legacy.ID).Scan(&scope))
	assert.Equal(t, 1, rows, "no scoped duplicate is created")
	assert.Equal(t, "acct", scope)
}

func TestIngestObservation_TrivialChangeUpdatesItemWithoutEvent(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	first, err := st.InboxItems.IngestObservation(ctx, activityClassifier("initial"), IngestObservationParams{
		ProfileID: "p", Topic: "source:p/a", Current: observation(`{"v":1}`),
	})
	require.NoError(t, err)

	current := observation(`{"v":2}`)
	current.ObservedAt = 101
	result, err := st.InboxItems.IngestObservation(ctx, testClassifier{func(_ *models.Observation, _ models.Observation) models.Classification {
		return models.Classification{Kind: "updated", Attention: models.AttentionTrivial, Transition: models.TransitionNone, Lifecycle: models.LifecycleActive, SourceState: "open"}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	require.True(t, result.Wrote)
	assert.EqualValues(t, 2, result.Revision)

	var revision, events int
	var payload []byte
	var sourceState string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT revision, payload, source_state FROM inbox_item WHERE id = ?`, first.ItemID).Scan(&revision, &payload, &sourceState))
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_event WHERE item_id = ?`, first.ItemID).Scan(&events))
	assert.Equal(t, 2, revision)
	assert.JSONEq(t, `{"v":2}`, string(payload))
	assert.Equal(t, "open", sourceState)
	assert.Equal(t, 1, events)
}

func TestIngestObservation_ConcurrentRevisionsAreMonotonic(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cur := observation(fmt.Sprintf(`{"v":%d}`, i))
			cur.ObservedAt = int64(i + 1)
			_, err := st.InboxItems.IngestObservation(ctx, activityClassifier(fmt.Sprintf("%d", i)), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: cur})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var revision int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT revision FROM inbox_item`).Scan(&revision))
	assert.Equal(t, 12, revision)
}

func TestApplyTransitionResurfacePolicies(t *testing.T) {
	now := int64(1)
	manual := ItemTriageState{ArchivedAt: &now, ArchivedActor: models.ArchivedActorManual.String()}
	activity := models.Classification{Attention: models.AttentionActivity}
	assert.NotNil(t, applyTransition(manual, activity, models.ResurfacePolicyStateChanges).ArchivedAt)
	assert.Nil(t, applyTransition(manual, activity, models.ResurfacePolicyAll).ArchivedAt)
	assert.NotNil(t, applyTransition(manual, activity, models.ResurfacePolicyNever).ArchivedAt)
	left := models.Classification{Transition: models.TransitionLeftTerminal, Attention: models.AttentionActivity}
	for _, policy := range []models.ResurfacePolicy{models.ResurfacePolicyAll, models.ResurfacePolicyStateChanges, models.ResurfacePolicyNever} {
		manualResult := applyTransition(manual, left, policy)
		if policy == models.ResurfacePolicyNever {
			assert.NotNil(t, manualResult.ArchivedAt)
		} else {
			assert.Nil(t, manualResult.ArchivedAt)
		}
		system := ItemTriageState{ArchivedAt: &now, ArchivedActor: models.ArchivedActorSystem.String()}
		got := applyTransition(system, left, policy)
		assert.Nil(t, got.ArchivedAt)
		assert.True(t, got.Unread)
	}
}

func TestIngestObservation_ManualArchiveIsNotClobberedByTerminalIngest(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	active := observation(`{"state":"open"}`)
	_, err := st.InboxItems.IngestObservation(ctx, activityClassifier("active"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: active})
	require.NoError(t, err)
	var id, revision int64
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT id, revision FROM inbox_item`).Scan(&id, &revision))
	_, err = db.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = 99, archived_actor = 'manual', revision = revision + 1 WHERE id = ? AND revision = ?`, id, revision)
	require.NoError(t, err)
	terminal := active
	terminal.Payload = []byte(`{"state":"closed"}`)
	_, err = st.InboxItems.IngestObservation(ctx, testClassifier{func(*models.Observation, models.Observation) models.Classification {
		return models.Classification{Kind: "closed", Transition: models.TransitionEnteredTerminal, Attention: models.AttentionActivity, Lifecycle: models.LifecycleTerminal}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: terminal})
	require.NoError(t, err)
	var actor string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT archived_actor FROM inbox_item WHERE id = ?`, id).Scan(&actor))
	assert.Equal(t, models.ArchivedActorManual.String(), actor)
}

func TestIngestObservation_RetainedManualArchiveGetsManualReason(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	current := observation(`{"v":1}`)
	_, err := st.InboxItems.IngestObservation(ctx, activityClassifier("initial"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	_, err = db.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = 99, archived_actor = 'manual', archived_reason = NULL`)
	require.NoError(t, err)

	current.Payload = []byte(`{"v":2}`)
	current.ObservedAt = 101
	_, err = st.InboxItems.IngestObservation(ctx, activityClassifier("later"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	var archivedAt int64
	var actor, reason string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT archived_at, archived_actor, archived_reason FROM inbox_item`).Scan(&archivedAt, &actor, &reason))
	assert.EqualValues(t, 99, archivedAt)
	assert.Equal(t, models.ArchivedActorManual.String(), actor)
	assert.Equal(t, models.ArchivedActorManual.String(), reason)
}

func TestIngestObservation_RetainedSystemArchivePreservesReason(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	current := observation(`{"state":"closed","v":1}`)
	_, err := st.InboxItems.IngestObservation(ctx, testClassifier{func(*models.Observation, models.Observation) models.Classification {
		return models.Classification{Kind: "closed", Transition: models.TransitionEnteredTerminal, Attention: models.AttentionActivity, Lifecycle: models.LifecycleTerminal, ArchivedReason: "closed"}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	current.Payload = []byte(`{"state":"closed","v":2}`)
	current.ObservedAt = 101
	_, err = st.InboxItems.IngestObservation(ctx, testClassifier{func(*models.Observation, models.Observation) models.Classification {
		return models.Classification{Kind: "updated", Attention: models.AttentionTrivial, Lifecycle: models.LifecycleTerminal}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	var actor, reason string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT archived_actor, archived_reason FROM inbox_item`).Scan(&actor, &reason))
	assert.Equal(t, models.ArchivedActorSystem.String(), actor)
	assert.Equal(t, "closed", reason)
}

func TestIngestObservation_BackfillsMissingOccurrenceKeyWithOffset(t *testing.T) {
	st, db := openTestStores(t)
	result, err := st.InboxItems.IngestObservation(t.Context(), activityClassifier(""), IngestObservationParams{
		ProfileID: "p",
		Topic:     "source:p/a",
		Current:   observation(`{"v":1}`),
	})
	require.NoError(t, err)
	require.True(t, result.Wrote)

	expected := fmt.Sprintf("%d", result.Offset)
	var occurrenceKey string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT occurrence_key FROM event_log WHERE "offset" = ?`, result.Offset).Scan(&occurrenceKey))
	assert.Equal(t, expected, occurrenceKey)

	msgs, _, err := st.EventLog.ReadFrom(t.Context(), 0, 1)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, expected, msgs[0].ID)
	assert.Equal(t, expected, msgs[0].OccurrenceKey, "the backfilled key is what a consumer dedups actions on")
}

func TestIngestObservation_NullOccurrenceDoesNotDeduplicate(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	for i := range 2 {
		cur := observation(fmt.Sprintf(`{"v":%d}`, i))
		cur.ObservedAt = int64(i + 1)
		_, err := st.InboxItems.IngestObservation(ctx, activityClassifier(""), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: cur})
		require.NoError(t, err)
	}
	var events int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT count(*) FROM inbox_event`).Scan(&events))
	assert.Equal(t, 2, events)
}

func TestIngestObservationBoundsDetail(t *testing.T) {
	st, db := openTestStores(t)
	detail := make([]byte, maxEventDetailBytes+100)
	_, err := st.InboxItems.IngestObservation(t.Context(), testClassifier{func(*models.Observation, models.Observation) models.Classification {
		return models.Classification{Kind: "activity", Attention: models.AttentionActivity, Lifecycle: models.LifecycleActive, Detail: detail}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: observation(`{"v":1}`)})
	require.NoError(t, err)
	var got []byte
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT detail FROM inbox_event`).Scan(&got))
	assert.LessOrEqual(t, len(got), maxEventDetailBytes)
}
