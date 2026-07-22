package pipelinedb

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testClassifier struct {
	classify func(*Observation, Observation) Classification
}

func (c testClassifier) Classify(prev *Observation, current Observation) Classification {
	return c.classify(prev, current)
}

func observation(payload string) Observation {
	return Observation{ExternalID: "acme/repo#1", Title: "one", URL: "https://example.test/1", SourceKind: "test", ObservedAt: 100, Payload: []byte(payload)}
}

func activityClassifier(key string) testClassifier {
	return testClassifier{func(_ *Observation, _ Observation) Classification {
		return Classification{Kind: "activity", Attention: AttentionActivity, Transition: TransitionNone, Lifecycle: LifecycleActive, OccurrenceKey: key, Summary: "activity"}
	}}
}

func TestIngestObservation_DuplicatePayloadWritesNothing(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	current := observation(`{"v":1}`)
	first, err := db.IngestObservation(ctx, activityClassifier("one"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	require.True(t, first.Wrote)
	second, err := db.IngestObservation(ctx, activityClassifier("one"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	assert.False(t, second.Wrote)
	var revision, events int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT revision FROM inbox_item`).Scan(&revision))
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT count(*) FROM inbox_event`).Scan(&events))
	assert.Equal(t, 1, revision)
	assert.Equal(t, 1, events)
}

func TestIngestObservation_TrivialChangeUpdatesItemWithoutEvent(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	first, err := db.IngestObservation(ctx, activityClassifier("initial"), IngestObservationParams{
		ProfileID: "p", Topic: "source:p/a", Current: observation(`{"v":1}`),
	})
	require.NoError(t, err)

	current := observation(`{"v":2}`)
	current.ObservedAt = 101
	result, err := db.IngestObservation(ctx, testClassifier{func(_ *Observation, _ Observation) Classification {
		return Classification{Kind: "updated", Attention: AttentionTrivial, Transition: TransitionNone, Lifecycle: LifecycleActive, SourceState: "open"}
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
	db := openTestDB(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cur := observation(fmt.Sprintf(`{"v":%d}`, i))
			cur.ObservedAt = int64(i + 1)
			_, err := db.IngestObservation(ctx, activityClassifier(fmt.Sprintf("%d", i)), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: cur})
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
	manual := ItemTriageState{ArchivedAt: &now, ArchivedActor: ArchivedActorManual.String()}
	activity := Classification{Attention: AttentionActivity}
	assert.NotNil(t, applyTransition(manual, activity, ResurfacePolicyStateChanges).ArchivedAt)
	assert.Nil(t, applyTransition(manual, activity, ResurfacePolicyAll).ArchivedAt)
	assert.NotNil(t, applyTransition(manual, activity, ResurfacePolicyNever).ArchivedAt)
	left := Classification{Transition: TransitionLeftTerminal, Attention: AttentionActivity}
	for _, policy := range []ResurfacePolicy{ResurfacePolicyAll, ResurfacePolicyStateChanges, ResurfacePolicyNever} {
		manualResult := applyTransition(manual, left, policy)
		if policy == ResurfacePolicyNever {
			assert.NotNil(t, manualResult.ArchivedAt)
		} else {
			assert.Nil(t, manualResult.ArchivedAt)
		}
		system := ItemTriageState{ArchivedAt: &now, ArchivedActor: ArchivedActorSystem.String()}
		got := applyTransition(system, left, policy)
		assert.Nil(t, got.ArchivedAt)
		assert.True(t, got.Unread)
	}
}

func TestIngestObservation_ManualArchiveIsNotClobberedByTerminalIngest(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	active := observation(`{"state":"open"}`)
	_, err := db.IngestObservation(ctx, activityClassifier("active"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: active})
	require.NoError(t, err)
	var id, revision int64
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT id, revision FROM inbox_item`).Scan(&id, &revision))
	_, err = db.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = 99, archived_actor = 'manual', revision = revision + 1 WHERE id = ? AND revision = ?`, id, revision)
	require.NoError(t, err)
	terminal := active
	terminal.Payload = []byte(`{"state":"closed"}`)
	_, err = db.IngestObservation(ctx, testClassifier{func(*Observation, Observation) Classification {
		return Classification{Kind: "closed", Transition: TransitionEnteredTerminal, Attention: AttentionActivity, Lifecycle: LifecycleTerminal}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: terminal})
	require.NoError(t, err)
	var actor string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT archived_actor FROM inbox_item WHERE id = ?`, id).Scan(&actor))
	assert.Equal(t, ArchivedActorManual.String(), actor)
}

func TestIngestObservation_RetainedManualArchiveGetsManualReason(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	current := observation(`{"v":1}`)
	_, err := db.IngestObservation(ctx, activityClassifier("initial"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	_, err = db.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = 99, archived_actor = 'manual', archived_reason = NULL`)
	require.NoError(t, err)

	current.Payload = []byte(`{"v":2}`)
	current.ObservedAt = 101
	_, err = db.IngestObservation(ctx, activityClassifier("later"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	var archivedAt int64
	var actor, reason string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT archived_at, archived_actor, archived_reason FROM inbox_item`).Scan(&archivedAt, &actor, &reason))
	assert.EqualValues(t, 99, archivedAt)
	assert.Equal(t, ArchivedActorManual.String(), actor)
	assert.Equal(t, ArchivedActorManual.String(), reason)
}

func TestIngestObservation_RetainedSystemArchivePreservesReason(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	current := observation(`{"state":"closed","v":1}`)
	_, err := db.IngestObservation(ctx, testClassifier{func(*Observation, Observation) Classification {
		return Classification{Kind: "closed", Transition: TransitionEnteredTerminal, Attention: AttentionActivity, Lifecycle: LifecycleTerminal, ArchivedReason: "closed"}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	current.Payload = []byte(`{"state":"closed","v":2}`)
	current.ObservedAt = 101
	_, err = db.IngestObservation(ctx, testClassifier{func(*Observation, Observation) Classification {
		return Classification{Kind: "updated", Attention: AttentionTrivial, Lifecycle: LifecycleTerminal}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	var actor, reason string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT archived_actor, archived_reason FROM inbox_item`).Scan(&actor, &reason))
	assert.Equal(t, ArchivedActorSystem.String(), actor)
	assert.Equal(t, "closed", reason)
}

func TestIngestObservation_BackfillsMissingOccurrenceKeyWithOffset(t *testing.T) {
	db := openTestDB(t)
	result, err := db.IngestObservation(t.Context(), activityClassifier(""), IngestObservationParams{
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

	msgs, next, err := db.ReadFrom(t.Context(), 0, 1)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, result.Offset, next)
	assert.NotEmpty(t, msgs[0].OccurrenceKey)
	assert.Equal(t, expected, msgs[0].OccurrenceKey)
}

func TestIngestObservation_NullOccurrenceDoesNotDeduplicate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for i := range 2 {
		cur := observation(fmt.Sprintf(`{"v":%d}`, i))
		cur.ObservedAt = int64(i + 1)
		_, err := db.IngestObservation(ctx, activityClassifier(""), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: cur})
		require.NoError(t, err)
	}
	var events int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT count(*) FROM inbox_event`).Scan(&events))
	assert.Equal(t, 2, events)
}

func TestIngestObservationBoundsDetail(t *testing.T) {
	db := openTestDB(t)
	detail := make([]byte, maxEventDetailBytes+100)
	_, err := db.IngestObservation(t.Context(), testClassifier{func(*Observation, Observation) Classification {
		return Classification{Kind: "activity", Attention: AttentionActivity, Lifecycle: LifecycleActive, Detail: detail}
	}}, IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: observation(`{"v":1}`)})
	require.NoError(t, err)
	var got []byte
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT detail FROM inbox_event`).Scan(&got))
	assert.LessOrEqual(t, len(got), maxEventDetailBytes)
}
