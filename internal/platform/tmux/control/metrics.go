package tmuxcc

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/internal/platform/observe"
)

// No instrument here is labelled by session slug or window id: both are
// unbounded per user, and which session was noisy is a span question.
var (
	meter = observe.Meter("/internal/app/tmuxcc")

	streamedBytes = observe.Must(meter.Int64Counter(
		"tmux.stream.bytes",
		metric.WithDescription("Bytes decoded from tmux %output and forwarded to the broker."),
		metric.WithUnit("By"),
	))

	// Boundaries reach defaultBufferBytes. The SDK default stops at 10 KB, which
	// puts every interesting backlog in the overflow bucket.
	bufferDepth = observe.Must(meter.Int64Histogram(
		"tmux.stream.buffer.depth",
		metric.WithDescription("Broker backlog depth measured after a publish."),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(
			1<<10, 4<<10, 16<<10, 64<<10, 256<<10, 1<<20, 2<<20, 4<<20, 8<<20,
		),
	))

	lifecycleTransitions = observe.Must(meter.Int64Counter(
		"tmux.stream.lifecycle",
		metric.WithDescription("Stream pause and resume transitions."),
	))

	// Seconds, and this path takes tens of microseconds. The SDK default starts
	// at 5, so every observation landed in the first bucket and a 92us p99
	// reported as 4.95s.
	frameLatency = observe.Must(meter.Float64Histogram(
		"tmux.stream.frame.latency",
		metric.WithDescription("Delay from tmux %output decode to the frame reaching the transport."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1,
		),
	))
)

// ObserveFrameLatency is exported because only the transport knows when a frame
// reached the wire.
func ObserveFrameLatency(ctx context.Context, d time.Duration) {
	frameLatency.Record(ctx, d.Seconds())
}

// Built once: metric.WithAttributes allocates.
var (
	statePaused  = metric.WithAttributes(attribute.String("state", "paused"))
	stateResumed = metric.WithAttributes(attribute.String("state", "resumed"))
)
