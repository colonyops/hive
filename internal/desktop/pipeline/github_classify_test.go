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
	reopened := classifier.Classify(&closed, previous)
	assert.Equal(t, pipelinedb.TransitionLeftTerminal, reopened.Transition)
}
