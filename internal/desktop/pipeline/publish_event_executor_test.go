package pipeline

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

// fakeEventPublisher records every Publish call.
type fakeEventPublisher struct {
	topics   []string
	payloads [][]byte
	err      error
}

func (f *fakeEventPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	f.topics = append(f.topics, topic)
	f.payloads = append(f.payloads, payload)
	return f.err
}

func TestPublishEventExecutor_PublishesRawPayloadToConfiguredTopic(t *testing.T) {
	pub := &fakeEventPublisher{}
	exec := NewPublishEventExecutor(pub)

	action := actions.Action{
		ID:     "notify",
		Type:   "publish-event",
		Config: &actions.PublishEventConfig{Topic: "pipeline.pr-events"},
	}
	raw := []byte(`{"title":"hello"}`)

	err := exec.Execute(t.Context(), action, OutputData{Payload: map[string]any{"title": "hello"}, Raw: raw})
	require.NoError(t, err)

	require.Len(t, pub.topics, 1)
	assert.Equal(t, "pipeline.pr-events", pub.topics[0])
	assert.JSONEq(t, string(raw), string(pub.payloads[0]))
}

func TestPublishEventExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewPublishEventExecutor(&fakeEventPublisher{})
	action := actions.Action{ID: "x", Type: "publish-event", Config: &actions.ShellConfig{}}

	err := exec.Execute(t.Context(), action, OutputData{})
	require.Error(t, err)
}

func TestNewPublishEventExecutor_NilPublisherDefaultsToLoggingStub(t *testing.T) {
	exec := NewPublishEventExecutor(nil)
	action := actions.Action{
		ID:     "notify",
		Type:   "publish-event",
		Config: &actions.PublishEventConfig{Topic: "pipeline.pr-events"},
	}
	require.NoError(t, exec.Execute(t.Context(), action, OutputData{Raw: []byte(`{}`)}))
}

func TestLoggingEventPublisher_ReturnsNil(t *testing.T) {
	p := LoggingEventPublisher{Logger: zerolog.Nop()}
	require.NoError(t, p.Publish(t.Context(), "topic", []byte(`{}`)))
}
