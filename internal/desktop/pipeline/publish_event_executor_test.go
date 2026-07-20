package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/eventbus"
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

type fakeInternalEventBus struct {
	events   []eventbus.Event
	payloads []any
	err      error
}

func (f *fakeInternalEventBus) Publish(event eventbus.Event, payload any) error {
	f.events = append(f.events, event)
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

func TestPublishEventExecutor_PropagatesPublishFailure(t *testing.T) {
	pub := &fakeEventPublisher{err: errors.New("event bus full")}
	exec := NewPublishEventExecutor(pub)
	action := actions.Action{ID: "notify", Type: "publish-event", Config: &actions.PublishEventConfig{Topic: "pipeline.pr-events"}}

	err := exec.Execute(t.Context(), action, OutputData{Raw: []byte(`{}`)})
	require.ErrorIs(t, err, pub.err)
}

func TestPublishEventExecutor_WrongConfigType_IsError(t *testing.T) {
	exec := NewPublishEventExecutor(&fakeEventPublisher{})
	action := actions.Action{ID: "x", Type: "publish-event", Config: &actions.ShellConfig{}}

	err := exec.Execute(t.Context(), action, OutputData{})
	require.Error(t, err)
}

func TestPublishEventExecutor_NilPublisherIsError(t *testing.T) {
	exec := NewPublishEventExecutor(nil)
	action := actions.Action{ID: "notify", Type: "publish-event", Config: &actions.PublishEventConfig{Topic: "pipeline.pr-events"}}
	require.Error(t, exec.Execute(t.Context(), action, OutputData{Raw: []byte(`{}`)}))
}

func TestEventBusPublisher_UsesConfiguredTopicAndCopiesPayload(t *testing.T) {
	bus := &fakeInternalEventBus{}
	publisher := NewEventBusPublisher(bus)
	payload := []byte(`{"title":"hello"}`)

	require.NoError(t, publisher.Publish(t.Context(), "pipeline.pr-events", payload))
	payload[0] = '['

	require.Equal(t, []eventbus.Event{"pipeline.pr-events"}, bus.events)
	require.Equal(t, []any{[]byte(`{"title":"hello"}`)}, bus.payloads)
}

func TestEventBusPublisher_PropagatesBackpressure(t *testing.T) {
	bus := &fakeInternalEventBus{err: eventbus.ErrFull}
	err := NewEventBusPublisher(bus).Publish(t.Context(), "pipeline.pr-events", []byte(`{}`))
	require.ErrorIs(t, err, eventbus.ErrFull)
}
