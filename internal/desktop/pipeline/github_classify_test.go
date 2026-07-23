package pipeline

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAbsenceConfirmer struct {
	called  bool
	verdict AbsenceVerdict
}

func (f *fakeAbsenceConfirmer) ConfirmAbsence(_ context.Context, _ Observation) (AbsenceVerdict, error) {
	f.called = true
	return f.verdict, nil
}

func TestGithubClassifierDelegatesAbsenceToInjectableConfirmer(t *testing.T) {
	fake := &fakeAbsenceConfirmer{verdict: AbsenceVerdict{Terminal: true}}
	classifier := newGithubClassifier(fake)
	verdict, err := classifier.ConfirmAbsence(t.Context(), Observation{ExternalID: "o/r#1"})
	require.NoError(t, err)
	assert.True(t, fake.called)
	assert.True(t, verdict.Terminal)
}

func TestGithubClassifierTerminalAndReopenTransitions(t *testing.T) {
	classifier := newGithubClassifier(&fakeAbsenceConfirmer{})
	previous := Observation{ExternalID: "o/r#1", Payload: []byte(`{"state":"open","updatedAt":1}`)}
	closed := Observation{ExternalID: "o/r#1", Payload: []byte(`{"state":"closed","updatedAt":2}`)}
	entered := classifier.Classify(&previous, closed)
	assert.Equal(t, pipelinedb.TransitionEnteredTerminal, entered.Transition)
	assert.Equal(t, "Closed", entered.Summary)
	reopened := classifier.Classify(&closed, previous)
	assert.Equal(t, pipelinedb.TransitionLeftTerminal, reopened.Transition)
	assert.Equal(t, "Reopened", reopened.Summary)
}

func TestGithubClassifierDescribesObservedActivity(t *testing.T) {
	classifier := newGithubClassifier(&fakeAbsenceConfirmer{})
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
			previous := Observation{ExternalID: "o/r#1", Payload: []byte(tt.previous)}
			got := classifier.Classify(&previous, Observation{ExternalID: "o/r#1", Payload: []byte(tt.current)})
			assert.Equal(t, tt.kind, got.Kind)
			assert.Equal(t, tt.summary, got.Summary)
			assert.Equal(t, pipelinedb.AttentionActivity, got.Attention)
			assert.NotEmpty(t, got.Detail)
		})
	}
}
