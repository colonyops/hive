package canonical

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func TestClassifier_FirstObservationActiveState(t *testing.T) {
	current := models.Observation{ExternalID: "x", Title: "t", ObservedAt: 100, Payload: []byte(`{"id":"x","state":"open"}`)}
	got := Classifier{}.Classify(nil, current)

	assert.Equal(t, "received", got.Kind)
	assert.Equal(t, models.TransitionNone, got.Transition)
	assert.Equal(t, models.AttentionActivity, got.Attention)
	assert.Equal(t, models.LifecycleActive, got.Lifecycle)
	assert.Equal(t, "open", got.SourceState)
	assert.Equal(t, "x@100", got.OccurrenceKey)
	assert.Empty(t, got.ArchivedReason)
}

func TestClassifier_MissingStateStaysActive(t *testing.T) {
	current := models.Observation{ExternalID: "x", Title: "t", ObservedAt: 100, Payload: []byte(`{"id":"x"}`)}
	got := Classifier{}.Classify(nil, current)
	assert.Equal(t, models.LifecycleActive, got.Lifecycle)
	assert.Empty(t, got.SourceState)

	nonObject := models.Observation{ExternalID: "y", Title: "t", ObservedAt: 100, Payload: []byte(`[1,2,3]`)}
	got = Classifier{}.Classify(nil, nonObject)
	assert.Equal(t, models.LifecycleActive, got.Lifecycle)
	assert.Empty(t, got.SourceState)
}

func TestClassifier_FirstObservationAlreadyTerminal(t *testing.T) {
	current := models.Observation{ExternalID: "x", Title: "t", ObservedAt: 100, Payload: []byte(`{"id":"x","state":"done"}`)}
	got := Classifier{}.Classify(nil, current)

	assert.Equal(t, "received", got.Kind)
	assert.Equal(t, models.LifecycleTerminal, got.Lifecycle)
	assert.Equal(t, models.TransitionNone, got.Transition, "first-seen terminal is not auto-archived, matching GitHub")
	assert.Equal(t, "done", got.SourceState)
	assert.Empty(t, got.ArchivedReason)
}

func TestClassifier_EntersTerminalCaseInsensitive(t *testing.T) {
	prev := models.Observation{ExternalID: "x", Payload: []byte(`{"id":"x","state":"open"}`)}
	current := models.Observation{ExternalID: "x", Title: "t", ObservedAt: 200, Payload: []byte(`{"id":"x","state":"Resolved"}`)}
	got := Classifier{}.Classify(&prev, current)

	assert.Equal(t, "resolved", got.Kind)
	assert.Equal(t, "Resolved", got.Summary)
	assert.Equal(t, models.TransitionEnteredTerminal, got.Transition)
	assert.Equal(t, models.AttentionActivity, got.Attention)
	assert.Equal(t, "resolved", got.ArchivedReason)
	assert.Equal(t, "resolved", got.SourceState)
	assert.Equal(t, models.LifecycleTerminal, got.Lifecycle)
}

func TestClassifier_LeavesTerminalReopens(t *testing.T) {
	prev := models.Observation{ExternalID: "x", Payload: []byte(`{"id":"x","state":"closed"}`)}
	current := models.Observation{ExternalID: "x", Title: "t", ObservedAt: 200, Payload: []byte(`{"id":"x","state":"open"}`)}
	got := Classifier{}.Classify(&prev, current)

	assert.Equal(t, "reopened", got.Kind)
	assert.Equal(t, "Reopened", got.Summary)
	assert.Equal(t, models.TransitionLeftTerminal, got.Transition)
	assert.Equal(t, models.LifecycleActive, got.Lifecycle)
	assert.Empty(t, got.ArchivedReason)
}

func TestClassifier_UnchangedActiveStateIsUpdated(t *testing.T) {
	prev := models.Observation{ExternalID: "x", Payload: []byte(`{"id":"x","n":1}`)}
	current := models.Observation{ExternalID: "x", Title: "t", ObservedAt: 200, Payload: []byte(`{"id":"x","n":2}`)}
	got := Classifier{}.Classify(&prev, current)

	assert.Equal(t, "updated", got.Kind)
	assert.Equal(t, current.Title, got.Summary)
	assert.Equal(t, models.TransitionNone, got.Transition)
	assert.Equal(t, models.LifecycleActive, got.Lifecycle)
}

// WithState is how a snapshot source archives an item that left it, so what it
// must not do is lose the fields the archived item still renders from.
func TestWithState_PreservesEveryOtherField(t *testing.T) {
	got := WithState([]byte(`{"id":"x","title":"Deploy failed","url":"https://x","state":"firing"}`), TerminalState)

	assert.JSONEq(t, `{"id":"x","title":"Deploy failed","url":"https://x","state":"resolved"}`, string(got))
	assert.True(t, isTerminal(State(got)))
}

func TestWithState_LeavesANonObjectPayloadAlone(t *testing.T) {
	assert.JSONEq(t, `[1,2,3]`, string(WithState([]byte(`[1,2,3]`), TerminalState)))
}
