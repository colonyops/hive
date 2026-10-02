package jobs

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
)

// Neither instrument is labelled by action id or target: both are per-user and
// unbounded, so which action ran is a question for the job row or the log line.
var (
	meter = observe.Meter("/internal/app/jobs")

	transitions = observe.Must(meter.Int64Counter(
		"job.transitions",
		metric.WithDescription("Job lifecycle transitions, by the status entered."),
	))

	// Seconds. The output worker polls every five seconds, so queue time alone
	// puts most runs past the SDK's first buckets, and a clone or a shell
	// action can run for minutes.
	duration = observe.Must(meter.Float64Histogram(
		"job.duration",
		metric.WithDescription("Seconds from a job's Begin to its Done or Fail."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600),
	))
)

// Built once per status: metric.WithAttributes allocates.
var statusAttrs = func() map[JobStatus]metric.MeasurementOption {
	names := JobStatusNames()
	out := make(map[JobStatus]metric.MeasurementOption, len(names))
	for _, name := range names {
		out[JobStatus(name)] = metric.WithAttributes(attribute.String("status", name))
	}
	return out
}()

// Metered counts every transition requested for a nonzero id, whether or not
// inner persisted it, and times a job from its Begin to its Done or Fail. A
// job whose Begin happened in another process (a Resume after a restart) is
// counted but not timed.
func Metered(inner Recorder) Recorder {
	return &metered{inner: inner, now: time.Now, started: map[int64]time.Time{}}
}

type metered struct {
	inner Recorder
	now   func() time.Time

	mu      sync.Mutex
	started map[int64]time.Time
}

func (m *metered) Begin(ctx context.Context, label, actionID, target string) int64 {
	id := m.inner.Begin(ctx, label, actionID, target)
	if id == 0 {
		return 0
	}
	m.mu.Lock()
	m.started[id] = m.now()
	m.mu.Unlock()
	transitions.Add(ctx, 1, statusAttrs[JobStatusQueued])
	return id
}

func (m *metered) Running(ctx context.Context, id int64, commandID int64) {
	m.inner.Running(ctx, id, commandID)
	if id != 0 {
		transitions.Add(ctx, 1, statusAttrs[JobStatusRunning])
	}
}

func (m *metered) Resume(ctx context.Context, commandID int64) int64 {
	return m.inner.Resume(ctx, commandID)
}

func (m *metered) Done(ctx context.Context, id int64) {
	m.inner.Done(ctx, id)
	m.finish(ctx, id, JobStatusDone)
}

func (m *metered) Fail(ctx context.Context, id int64, reason string) {
	m.inner.Fail(ctx, id, reason)
	m.finish(ctx, id, JobStatusFailed)
}

func (m *metered) finish(ctx context.Context, id int64, status JobStatus) {
	if id == 0 {
		return
	}
	transitions.Add(ctx, 1, statusAttrs[status])
	m.mu.Lock()
	began, seen := m.started[id]
	delete(m.started, id)
	m.mu.Unlock()
	if seen {
		duration.Record(ctx, m.now().Sub(began).Seconds(), statusAttrs[status])
	}
}
