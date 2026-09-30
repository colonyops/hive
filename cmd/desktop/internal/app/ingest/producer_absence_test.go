package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
)

// capableInstance is one instance that declares the classification and
// absence capabilities, wired the way a real connector's factory wires them.
// The producer reads them straight off the instance — there is no assertion
// to miss and no adapter map to look up by source kind.
func capableInstance(flowID, nodeID string, pull connector.PullSource, classifier models.Classifier, absence models.AbsenceConfirmer) connector.Instance {
	return connector.Instance{
		Type: "sources.test",
		Node: connector.Node{FlowID: flowID, NodeID: nodeID},
		Metadata: connector.Metadata{
			ProfileID:  flowID,
			SourceKind: "github",
			Policy:     models.ResurfacePolicyStateChanges,
		},
		Pull:       pull,
		Classifier: classifier,
		Absence:    absence,
	}
}

type countingAbsence struct{ calls atomic.Int32 }

func (c *countingAbsence) ConfirmAbsence(context.Context, []models.Observation) (map[string]models.AbsenceVerdict, error) {
	c.calls.Add(1)
	return map[string]models.AbsenceVerdict{}, nil
}

type payloadHydratingAbsence struct {
	calls      int
	batchSizes []int
	observed   models.Observation
	updatedAt  int64
	terminal   bool
}

func (c *payloadHydratingAbsence) ConfirmAbsence(_ context.Context, previous []models.Observation) (map[string]models.AbsenceVerdict, error) {
	c.calls++
	c.batchSizes = append(c.batchSizes, len(previous))
	if len(previous) > 0 {
		c.observed = previous[0]
	}
	verdicts := make(map[string]models.AbsenceVerdict, len(previous))
	for _, prev := range previous {
		var item feed.Item
		if err := json.Unmarshal(prev.Payload, &item); err != nil {
			return nil, err
		}
		item.UpdatedAt = c.updatedAt
		payload, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		current := prev
		current.Payload = payload
		current.ObservedAt = item.UpdatedAt
		verdicts[prev.ExternalID] = models.AbsenceVerdict{Current: &current, Terminal: c.terminal}
	}
	return verdicts, nil
}

type activeAbsenceClassifier struct{}

func (activeAbsenceClassifier) Classify(_ *models.Observation, current models.Observation) models.Classification {
	return models.Classification{
		Kind: "updated", Attention: models.AttentionTrivial,
		Transition: models.TransitionNone, Lifecycle: models.LifecycleActive,
		Summary: current.Title,
	}
}

func TestProducerAbsenceIsScopedToExactSourceTopic(t *testing.T) {
	db := openTestPipelineDB(t)
	classifier := genericClassifier{}
	_, err := stores.New(db, stores.Options{}).InboxItems.IngestObservation(t.Context(), classifier, stores.IngestObservationParams{ProfileID: "profile", Topic: "source:profile/second", Current: models.Observation{ExternalID: "only-second", SourceKind: "github", Payload: []byte(`{"v":1}`), ObservedAt: 1}})
	require.NoError(t, err)
	absence := &countingAbsence{}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "first", &fakeSource{}, classifier, absence),
	}}, time.Hour, nil, zerolog.Nop())
	producer.Tick(t.Context())
	assert.Zero(t, absence.calls.Load(), "a sibling source topic must not be considered absent")
}

func TestProducerAbsenceHydrationPreservesInboxMetadata(t *testing.T) {
	db := openTestPipelineDB(t)
	item := feed.Item{ID: "acme/repo#1", Title: "Keep this title", URL: "https://example.test/acme/repo/issues/1", UpdatedAt: 100}
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	src := &fakeSource{batches: [][]Msg{{{
		Topic: "source:profile/source", Key: item.ID, Payload: payload,
	}}}}
	absence := &payloadHydratingAbsence{updatedAt: 200, terminal: true}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, genericClassifier{}, absence),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())

	require.Equal(t, 1, absence.calls)
	assert.Equal(t, item.Title, absence.observed.Title)
	assert.Equal(t, item.URL, absence.observed.URL)
	assert.Equal(t, item.UpdatedAt, absence.observed.ObservedAt)
	var title, url string
	var lastEventAt int64
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT title, url, last_event_at FROM inbox_item`).Scan(&title, &url, &lastEventAt))
	assert.Equal(t, item.Title, title)
	assert.Equal(t, item.URL, url)
	assert.Equal(t, int64(200), lastEventAt)
}

func TestProducerIngestsNonTerminalAbsenceConfirmation(t *testing.T) {
	db := openTestPipelineDB(t)
	item := feed.Item{ID: "acme/repo#1", Title: "Still active", URL: "https://example.test/acme/repo/issues/1", State: "open", UpdatedAt: 100}
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	src := &fakeSource{batches: [][]Msg{{{
		Topic: "source:profile/source", Key: item.ID, Payload: payload,
	}}}}
	absence := &payloadHydratingAbsence{updatedAt: 200, terminal: false}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, activeAbsenceClassifier{}, absence),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())

	require.Equal(t, 1, absence.calls)
	var lifecycle string
	var archivedAt *int64
	var lastEventAt int64
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT lifecycle, archived_at, last_event_at FROM inbox_item`).Scan(&lifecycle, &archivedAt, &lastEventAt))
	assert.Equal(t, models.LifecycleActive.String(), lifecycle)
	assert.Nil(t, archivedAt)
	assert.Equal(t, int64(200), lastEventAt)
}

func TestProducerConfirmsAbsentItemsInOneCall(t *testing.T) {
	db := openTestPipelineDB(t)
	const absentCount = 250
	batch := make([]Msg, absentCount)
	for i := range absentCount {
		item := feed.Item{ID: fmt.Sprintf("acme/repo#%d", i+1), Title: fmt.Sprintf("item %d", i+1), UpdatedAt: 100}
		payload, err := json.Marshal(item)
		require.NoError(t, err)
		batch[i] = Msg{Topic: "source:profile/source", Key: item.ID, Payload: payload}
	}
	src := &fakeSource{batches: [][]Msg{batch}}
	absence := &payloadHydratingAbsence{updatedAt: 200, terminal: false}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, genericClassifier{}, absence),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())

	require.Len(t, absence.batchSizes, 1, "one ConfirmAbsence call per tick regardless of how many keys are absent")
	assert.Equal(t, absentCount, absence.batchSizes[0])
}

type partialAbsence struct {
	verdicts map[string]models.AbsenceVerdict
}

func (p partialAbsence) ConfirmAbsence(context.Context, []models.Observation) (map[string]models.AbsenceVerdict, error) {
	return p.verdicts, errors.New("boom")
}

func TestProducerIngestsPartialAbsenceBatch(t *testing.T) {
	db := openTestPipelineDB(t)
	itemA := feed.Item{ID: "acme/repo#1", Title: "A", UpdatedAt: 100}
	payloadA, err := json.Marshal(itemA)
	require.NoError(t, err)
	itemB := feed.Item{ID: "acme/repo#2", Title: "B", UpdatedAt: 100}
	payloadB, err := json.Marshal(itemB)
	require.NoError(t, err)
	src := &fakeSource{batches: [][]Msg{{
		{Topic: "source:profile/source", Key: itemA.ID, Payload: payloadA},
		{Topic: "source:profile/source", Key: itemB.ID, Payload: payloadB},
	}}}

	currentA := models.Observation{ExternalID: itemA.ID, SourceKind: "github", Title: "A updated", Payload: payloadA, ObservedAt: 200}
	currentB := models.Observation{ExternalID: itemB.ID, SourceKind: "github", Title: "B updated", Payload: payloadB, ObservedAt: 200}
	absence := partialAbsence{verdicts: map[string]models.AbsenceVerdict{
		itemA.ID: {Current: &currentA},
		itemB.ID: {Current: &currentB},
	}}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, genericClassifier{}, absence),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())

	var count int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM inbox_item`).Scan(&count))
	assert.Equal(t, 2, count, "both resolved verdicts are ingested despite the confirmer's error")
}

type emptyAbsence struct{}

func (emptyAbsence) ConfirmAbsence(context.Context, []models.Observation) (map[string]models.AbsenceVerdict, error) {
	return map[string]models.AbsenceVerdict{}, nil
}

func TestProducerKeepsNotFoundItems(t *testing.T) {
	db := openTestPipelineDB(t)
	item := feed.Item{ID: "acme/repo#1", Title: "A", UpdatedAt: 100}
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	src := &fakeSource{batches: [][]Msg{{
		{Topic: "source:profile/source", Key: item.ID, Payload: payload},
	}}}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, genericClassifier{}, emptyAbsence{}),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())

	var count int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM inbox_item`).Scan(&count))
	assert.Equal(t, 1, count, "an item with no verdict is not re-ingested")

	var archivedAt *int64
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT archived_at FROM inbox_item`).Scan(&archivedAt))
	assert.Nil(t, archivedAt, "an item with no verdict is not archived")

	var headCount int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM source_head WHERE topic = ? AND key = ?`, "source:profile/source", item.ID).Scan(&headCount))
	assert.Equal(t, 1, headCount, "source_head keeps the item when the confirmer has no answer")
}

// recordingAbsence records every batch it is asked to confirm and, when
// answer is set, echoes each item's own payload back as its verdict.
type recordingAbsence struct {
	calls      int
	batchSizes []int
	batchKeys  [][]string
	answer     bool
	terminal   bool
}

func (r *recordingAbsence) ConfirmAbsence(_ context.Context, previous []models.Observation) (map[string]models.AbsenceVerdict, error) {
	r.calls++
	keys := make([]string, len(previous))
	verdicts := make(map[string]models.AbsenceVerdict, len(previous))
	for i, prev := range previous {
		keys[i] = prev.ExternalID
		if r.answer {
			current := prev
			verdicts[prev.ExternalID] = models.AbsenceVerdict{Current: &current, Terminal: r.terminal}
		}
	}
	r.batchSizes = append(r.batchSizes, len(previous))
	r.batchKeys = append(r.batchKeys, keys)
	return verdicts, nil
}

func TestProducerConfirmsActiveAbsentItem(t *testing.T) {
	db := openTestPipelineDB(t)
	item := feed.Item{ID: "acme/repo#1", Title: "A", UpdatedAt: 100}
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	src := &fakeSource{batches: [][]Msg{{
		{Topic: "source:profile/source", Key: item.ID, Payload: payload},
	}}}
	absence := &recordingAbsence{}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, genericClassifier{}, absence),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context()) // tick 1: the item is observed, nothing absent yet
	producer.Tick(t.Context()) // tick 2: the source no longer emits it

	require.Equal(t, 1, absence.calls, "a non-archived, non-pruned absent item must reach the confirmer")
	require.Len(t, absence.batchKeys, 1)
	assert.Contains(t, absence.batchKeys[0], item.ID)
}

func TestProducerSkipsArchivedItemsInAbsence(t *testing.T) {
	db := openTestPipelineDB(t)
	item := feed.Item{ID: "acme/repo#1", Title: "A", UpdatedAt: 100}
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	src := &fakeSource{batches: [][]Msg{{
		{Topic: "source:profile/source", Key: item.ID, Payload: payload},
	}}}
	absence := &recordingAbsence{}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, genericClassifier{}, absence),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context()) // tick 1: item ingested

	_, err = db.Conn().ExecContext(t.Context(), `UPDATE inbox_item SET archived_at = 1, archived_actor = 'manual' WHERE external_id = ?`, item.ID)
	require.NoError(t, err)

	producer.Tick(t.Context()) // tick 2: source still doesn't emit it, but it's archived

	assert.Zero(t, absence.calls, "an archived item's key must never reach the confirmer")
}

func TestProducerStopsConfirmingTerminalItems(t *testing.T) {
	db := openTestPipelineDB(t)
	payload := []byte(`{"id":"acme/repo#1","title":"A"}`)
	_, err := stores.New(db, stores.Options{}).InboxItems.IngestObservation(t.Context(), genericClassifier{}, stores.IngestObservationParams{
		ProfileID: "profile", Topic: "source:profile/source",
		Current: models.Observation{ExternalID: "acme/repo#1", Title: "A", SourceKind: "github", ObservedAt: 100, Payload: payload},
	})
	require.NoError(t, err)

	src := &fakeSource{} // never emits: every tick treats the seeded item as absent
	absence := &recordingAbsence{answer: true, terminal: true}
	producer := newTestProducer(db, stubSources{instances: []connector.Instance{
		capableInstance("profile", "source", src, genericClassifier{}, absence),
	}}, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())

	require.Equal(t, 1, absence.calls)
	require.Len(t, absence.batchKeys, 1)
	assert.Contains(t, absence.batchKeys[0], "acme/repo#1")

	var headCount int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM source_head WHERE topic = ? AND key = ?`, "source:profile/source", "acme/repo#1").Scan(&headCount))
	assert.Zero(t, headCount, "a terminal verdict evicts the head row even though IngestObservation short-circuited on Wrote:false")

	producer.Tick(t.Context())

	assert.Equal(t, 1, absence.calls, "tick 2 finds no active head keys, so the confirmer is never called again")
}
