package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicEventPublishAndSubscribe(t *testing.T) {
	bus := New(1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	received := make(chan any, 1)
	bus.Subscribe("pipeline.pr-events", func(payload any) { received <- payload })
	go bus.Start(ctx)

	require.NoError(t, bus.Publish("pipeline.pr-events", []byte(`{"title":"hello"}`)))
	select {
	case payload := <-received:
		data, ok := payload.([]byte)
		require.True(t, ok, "dynamic event payload type = %T, want []byte", payload)
		assert.JSONEq(t, `{"title":"hello"}`, string(data))
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dynamic event")
	}
}

func TestDynamicEventPublishReportsBackpressure(t *testing.T) {
	bus := New(1)
	require.NoError(t, bus.Publish("pipeline.pr-events", nil))
	require.ErrorIs(t, bus.Publish("pipeline.pr-events", nil), ErrFull)
}
