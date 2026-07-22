package pipeline

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type metadataFakeSource struct {
	*fakeSource
	meta sourceMetadata
}

func (s metadataFakeSource) ingestMetadata() sourceMetadata { return s.meta }

type countingAbsence struct{ calls atomic.Int32 }

func (c *countingAbsence) ConfirmAbsence(context.Context, Observation) (AbsenceVerdict, error) {
	c.calls.Add(1)
	return AbsenceVerdict{}, nil
}

type payloadHydratingAbsence struct {
	calls     int
	observed  Observation
	updatedAt int64
	terminal  bool
}

func (c *payloadHydratingAbsence) ConfirmAbsence(_ context.Context, prev Observation) (AbsenceVerdict, error) {
	c.calls++
	c.observed = prev
	var item feed.Item
	if err := json.Unmarshal(prev.Payload, &item); err != nil {
		return AbsenceVerdict{}, err
	}
	item.UpdatedAt = c.updatedAt
	payload, err := json.Marshal(item)
	if err != nil {
		return AbsenceVerdict{}, err
	}
	current := prev
	current.Payload = payload
	current.ObservedAt = item.UpdatedAt
	return AbsenceVerdict{Current: &current, Terminal: c.terminal}, nil
}

type activeAbsenceClassifier struct{}

func (activeAbsenceClassifier) Classify(_ *Observation, current Observation) Classification {
	return Classification{
		Kind: "updated", Attention: pipelinedb.AttentionTrivial,
		Transition: pipelinedb.TransitionNone, Lifecycle: pipelinedb.LifecycleActive,
		Summary: current.Title,
	}
}

func TestProducerAbsenceIsScopedToExactSourceTopic(t *testing.T) {
	db := openTestPipelineDB(t)
	classifier := genericClassifier{}
	_, err := db.IngestObservation(t.Context(), classifier, pipelinedb.IngestObservationParams{ProfileID: "profile", Topic: "source:profile/second", Current: pipelinedb.Observation{ExternalID: "only-second", SourceKind: "github", Payload: []byte(`{"v":1}`), ObservedAt: 1}})
	require.NoError(t, err)
	absence := &countingAbsence{}
	producer := NewProducer(db, listerOf(map[string]Source{"profile/first": metadataFakeSource{fakeSource: &fakeSource{}, meta: sourceMetadata{ProfileID: "profile", SourceKind: "github", Policy: pipelinedb.ResurfacePolicyStateChanges}}}), time.Hour, nil, zerolog.Nop())
	producer.SetSourceAdapter(SourceAdapter{SourceKind: "github", Classifier: classifier, AbsenceConfirmer: absence})
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
	producer := NewProducer(db, listerOf(map[string]Source{
		"profile/source": metadataFakeSource{fakeSource: src, meta: sourceMetadata{ProfileID: "profile", SourceKind: "github", Policy: pipelinedb.ResurfacePolicyStateChanges}},
	}), time.Hour, nil, zerolog.Nop())
	producer.SetSourceAdapter(SourceAdapter{SourceKind: "github", Classifier: genericClassifier{}, AbsenceConfirmer: absence})

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
	producer := NewProducer(db, listerOf(map[string]Source{
		"profile/source": metadataFakeSource{fakeSource: src, meta: sourceMetadata{ProfileID: "profile", SourceKind: "github", Policy: pipelinedb.ResurfacePolicyStateChanges}},
	}), time.Hour, nil, zerolog.Nop())
	producer.SetSourceAdapter(SourceAdapter{SourceKind: "github", Classifier: activeAbsenceClassifier{}, AbsenceConfirmer: absence})

	producer.Tick(t.Context())
	producer.Tick(t.Context())

	require.Equal(t, 1, absence.calls)
	var lifecycle string
	var archivedAt *int64
	var lastEventAt int64
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT lifecycle, archived_at, last_event_at FROM inbox_item`).Scan(&lifecycle, &archivedAt, &lastEventAt))
	assert.Equal(t, pipelinedb.LifecycleActive.String(), lifecycle)
	assert.Nil(t, archivedAt)
	assert.Equal(t, int64(200), lastEventAt)
}
