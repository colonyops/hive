package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
)

func TestGithubClassifierTerminalAndReopenTransitions(t *testing.T) {
	classifier := classifier{}
	previous := models.Observation{ExternalID: "o/r#1", Payload: []byte(`{"state":"open","updatedAt":1}`)}
	closed := models.Observation{ExternalID: "o/r#1", Payload: []byte(`{"state":"closed","updatedAt":2}`)}
	entered := classifier.Classify(&previous, closed)
	assert.Equal(t, models.TransitionEnteredTerminal, entered.Transition)
	assert.Equal(t, "Closed", entered.Summary)
	reopened := classifier.Classify(&closed, previous)
	assert.Equal(t, models.TransitionLeftTerminal, reopened.Transition)
	assert.Equal(t, "Reopened", reopened.Summary)
}

type stubTerminalConfirmer struct {
	t        *testing.T
	wantRefs []feed.AbsentRef
	states   []ghclient.ItemState
}

func (s *stubTerminalConfirmer) ConfirmTerminal(_ context.Context, refs []feed.AbsentRef) ([]ghclient.ItemState, error) {
	s.t.Helper()
	assert.Equal(s.t, s.wantRefs, refs)
	return s.states, nil
}

func TestAbsenceConfirmer_KeysVerdictsByExternalID(t *testing.T) {
	mustPayload := func(item feed.Item) []byte {
		b, err := json.Marshal(item)
		require.NoError(t, err)
		return b
	}

	previous := []models.Observation{
		{ExternalID: "a-undecodable", Payload: []byte("not json")},
		{ExternalID: "b-zero-num", Payload: mustPayload(feed.Item{Repo: "acme/repo", Num: 0})},
		{ExternalID: "c-no-slash", Payload: mustPayload(feed.Item{Repo: "acme", Num: 5})},
		{ExternalID: "d-not-found", Payload: mustPayload(feed.Item{Repo: "acme/repo", Num: 10})},
		{ExternalID: "e-closed", Payload: mustPayload(feed.Item{Repo: "acme/repo", Num: 20})},
		{ExternalID: "e-open", Payload: mustPayload(feed.Item{Repo: "acme/repo", Num: 30})},
	}

	updatedAt := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	stub := &stubTerminalConfirmer{
		t: t,
		wantRefs: []feed.AbsentRef{
			{Repo: "acme/repo", Num: 10},
			{Repo: "acme/repo", Num: 20},
			{Repo: "acme/repo", Num: 30},
		},
		states: []ghclient.ItemState{
			{Found: false},
			{Found: true, State: "closed", UpdatedAt: updatedAt},
			{Found: true, State: "open", UpdatedAt: updatedAt},
		},
	}
	confirmer := &absenceConfirmer{live: stub}

	verdicts, err := confirmer.ConfirmAbsence(t.Context(), previous)
	require.NoError(t, err)

	require.Len(t, verdicts, 2)
	closedVerdict, ok := verdicts["e-closed"]
	require.True(t, ok)
	assert.True(t, closedVerdict.Terminal)
	openVerdict, ok := verdicts["e-open"]
	require.True(t, ok)
	assert.False(t, openVerdict.Terminal)

	for _, id := range []string{"a-undecodable", "b-zero-num", "c-no-slash", "d-not-found"} {
		_, ok := verdicts[id]
		assert.False(t, ok, "external id %q must have no verdict", id)
	}
}

func TestGithubClassifierDescribesObservedActivity(t *testing.T) {
	classifier := classifier{}
	tests := []struct {
		name, previous, current, kind, summary string
	}{
		{"comment", `{"state":"open","updatedAt":1,"labels":["bug"]}`, `{"state":"open","updatedAt":2,"reason":"comment"}`, "comment", "New comment activity"},
		{"review", `{"state":"open","updatedAt":1}`, `{"state":"open","updatedAt":2,"reason":"review_requested"}`, "review_requested", "Review requested"},
		{"labels", `{"state":"open","updatedAt":1,"labels":["bug"]}`, `{"state":"open","updatedAt":2,"labels":["bug","urgent"]}`, "labels", "Labels added: urgent"},
		{"generic", `{"state":"open","updatedAt":1}`, `{"state":"open","updatedAt":2}`, "updated", "Updated on GitHub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := models.Observation{ExternalID: "o/r#1", Payload: []byte(tt.previous)}
			got := classifier.Classify(&previous, models.Observation{ExternalID: "o/r#1", Payload: []byte(tt.current)})
			assert.Equal(t, tt.kind, got.Kind)
			assert.Equal(t, tt.summary, got.Summary)
			assert.Equal(t, models.AttentionActivity, got.Attention)
			assert.NotEmpty(t, got.Detail)
		})
	}
}
