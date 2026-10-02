package jobs

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// The global MeterProvider delegates exactly once, so the instruments in
// metrics.go bind to the first provider registered. One reader serves the
// package; a per-test provider would collect nothing.
var reader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	reader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	os.Exit(m.Run())
}

// fakeRecorder hands out the ids it is told to and remembers every call, so a
// test can assert the decorator forwarded what it was given.
type fakeRecorder struct {
	beginID  int64
	resumeID int64
	calls    []string
}

func (f *fakeRecorder) Begin(_ context.Context, label, actionID, target string) int64 {
	f.calls = append(f.calls, fmt.Sprintf("begin %s %s %s", label, actionID, target))
	return f.beginID
}

func (f *fakeRecorder) Running(_ context.Context, id int64, commandID int64) {
	f.calls = append(f.calls, fmt.Sprintf("running %d %d", id, commandID))
}

func (f *fakeRecorder) Resume(_ context.Context, commandID int64) int64 {
	f.calls = append(f.calls, fmt.Sprintf("resume %d", commandID))
	return f.resumeID
}

func (f *fakeRecorder) Done(_ context.Context, id int64) {
	f.calls = append(f.calls, fmt.Sprintf("done %d", id))
}

func (f *fakeRecorder) Fail(_ context.Context, id int64, reason string) {
	f.calls = append(f.calls, fmt.Sprintf("fail %d %s", id, reason))
}

type clock struct{ at time.Time }

func (c *clock) now() time.Time          { return c.at }
func (c *clock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newMetered(inner Recorder, c *clock) *metered {
	return &metered{inner: inner, now: c.now, started: map[int64]time.Time{}}
}

// Every instrument is cumulative for the life of the process, so a test reads
// before and after and asserts on the difference. The tests share the
// instruments and must not run in parallel.
type counts struct {
	queued, running, done, failed int64
	doneSamples, failedSamples    uint64
	doneSeconds, failedSeconds    float64
}

func (c counts) minus(o counts) counts {
	return counts{
		queued: c.queued - o.queued, running: c.running - o.running,
		done: c.done - o.done, failed: c.failed - o.failed,
		doneSamples: c.doneSamples - o.doneSamples, failedSamples: c.failedSamples - o.failedSamples,
		doneSeconds: c.doneSeconds - o.doneSeconds, failedSeconds: c.failedSeconds - o.failedSeconds,
	}
}

func readCounts(t *testing.T) counts {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	var c counts
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "job.transitions":
				sum, ok := m.Data.(metricdata.Sum[int64])
				require.True(t, ok)
				for _, dp := range sum.DataPoints {
					switch statusOf(dp.Attributes) {
					case JobStatusQueued:
						c.queued += dp.Value
					case JobStatusRunning:
						c.running += dp.Value
					case JobStatusDone:
						c.done += dp.Value
					case JobStatusFailed:
						c.failed += dp.Value
					}
				}
			case "job.duration":
				hist, ok := m.Data.(metricdata.Histogram[float64])
				require.True(t, ok)
				for _, dp := range hist.DataPoints {
					status := statusOf(dp.Attributes)
					if status == JobStatusDone {
						c.doneSamples += dp.Count
						c.doneSeconds += dp.Sum
					}
					if status == JobStatusFailed {
						c.failedSamples += dp.Count
						c.failedSeconds += dp.Sum
					}
				}
			}
		}
	}
	return c
}

func statusOf(set attribute.Set) JobStatus {
	value, _ := set.Value("status")
	return JobStatus(value.AsString())
}

func TestMetered_CountsEachTransitionAndTimesBeginToDone(t *testing.T) {
	inner := &fakeRecorder{beginID: 7}
	c := &clock{at: time.Unix(1_000, 0)}
	m := newMetered(inner, c)
	before := readCounts(t)

	id := m.Begin(t.Context(), "Review PR", "review", "pr-1")
	require.Equal(t, int64(7), id)
	c.advance(2 * time.Second)
	m.Running(t.Context(), id, 44)
	c.advance(3 * time.Second)
	m.Done(t.Context(), id)

	got := readCounts(t).minus(before)
	assert.Equal(t, counts{queued: 1, running: 1, done: 1, doneSamples: 1, doneSeconds: 5}, got)
	assert.Equal(t, []string{"begin Review PR review pr-1", "running 7 44", "done 7"}, inner.calls)
	assert.Empty(t, m.started, "a finished job releases its start time")
}

func TestMetered_TimesAFailedJobUnderItsOwnStatus(t *testing.T) {
	inner := &fakeRecorder{beginID: 8}
	c := &clock{at: time.Unix(1_000, 0)}
	m := newMetered(inner, c)
	before := readCounts(t)

	id := m.Begin(t.Context(), "Deploy", "deploy", "pr-2")
	c.advance(1500 * time.Millisecond)
	m.Fail(t.Context(), id, "boom")

	got := readCounts(t).minus(before)
	assert.Equal(t, counts{queued: 1, failed: 1, failedSamples: 1, failedSeconds: 1.5}, got)
	assert.Equal(t, []string{"begin Deploy deploy pr-2", "fail 8 boom"}, inner.calls)
	assert.Empty(t, m.started)
}

func TestMetered_IgnoresAJobTheRecorderRejected(t *testing.T) {
	inner := &fakeRecorder{beginID: 0}
	c := &clock{at: time.Unix(1_000, 0)}
	m := newMetered(inner, c)
	before := readCounts(t)

	id := m.Begin(t.Context(), "Review PR", "review", "pr-1")
	require.Zero(t, id)
	m.Running(t.Context(), id, 44)
	m.Done(t.Context(), id)
	m.Fail(t.Context(), id, "boom")

	assert.Equal(t, counts{}, readCounts(t).minus(before))
	assert.Empty(t, m.started)
}

func TestMetered_CountsButDoesNotTimeAResumedJob(t *testing.T) {
	inner := &fakeRecorder{resumeID: 9}
	c := &clock{at: time.Unix(1_000, 0)}
	m := newMetered(inner, c)
	before := readCounts(t)

	id := m.Resume(t.Context(), 44)
	require.Equal(t, int64(9), id)
	c.advance(time.Minute)
	m.Done(t.Context(), id)

	got := readCounts(t).minus(before)
	assert.Equal(t, counts{done: 1}, got, "the Begin happened in another process, so there is no duration to sample")
	assert.Equal(t, []string{"resume 44", "done 9"}, inner.calls)
}
