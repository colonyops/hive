package procstats

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

var reader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	reader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	os.Exit(m.Run())
}

func TestSampleReadsThisProcess(t *testing.T) {
	stats, err := New(int32(os.Getpid())).Sample(t.Context())
	require.NoError(t, err)

	require.Equal(t, int32(os.Getpid()), stats.Process.PID)
	require.NotZero(t, stats.Process.RSSBytes, "a running process holds resident memory")
	require.Positive(t, stats.Go.Goroutines)
	require.Equal(t, len(stats.Children) >= maxProcesses, stats.ChildrenTruncated)
}

func TestTotalsIncludeTheProcessTree(t *testing.T) {
	stats := Stats{
		Process:  Process{RSSBytes: 100, CPUPercent: 1.5},
		Children: []Process{{RSSBytes: 20, CPUPercent: 0.5}, {RSSBytes: 5, CPUPercent: 2}},
	}
	require.Equal(t, uint64(125), stats.TotalRSSBytes())
	require.InDelta(t, 4.0, stats.TotalCPUPercent(), 0.0001)
}

func TestPercentIgnoresImpossibleWindows(t *testing.T) {
	require.InDelta(t, 50.0, percent(0.5, 1), 0.0001)
	require.InDelta(t, 200.0, percent(4, 2), 0.0001, "a process on several cores exceeds one core's worth")
	require.Zero(t, percent(1, 0), "a zero-length window is not a rate")
	require.Zero(t, percent(-1, 1), "a counter that went backwards is not a rate")
}

// A sampler that outlives the processes it has seen must not keep their marks:
// a terminal-heavy session churns short-lived shells all day.
func TestSamplerForgetsProcessesItNoLongerSees(t *testing.T) {
	s := New(int32(os.Getpid()))
	s.marks[999999] = cpuMark{seconds: 1}

	_, err := s.Sample(t.Context())
	require.NoError(t, err)
	require.NotContains(t, s.marks, int32(999999))
	require.Contains(t, s.marks, s.pid)
}

func TestObservableGaugesSampleOnCollection(t *testing.T) {
	calls := 0
	registration := registerMetrics(func(context.Context) (Stats, error) {
		calls++
		return Stats{
			Process: Process{RSSBytes: 100, CPUPercent: 12.5},
			Children: []Process{
				{RSSBytes: 20, CPUPercent: 3},
				{RSSBytes: 5, CPUPercent: 4},
			},
			ChildrenTruncated: true,
		}, nil
	})
	t.Cleanup(func() { require.NoError(t, registration.Unregister()) })

	require.Zero(t, calls)
	metrics := collectMetrics(t)
	require.Equal(t, 1, calls)

	assert.Equal(t, int64(100), intGaugeValue(t, metrics, "process.memory.rss", "self"))
	assert.Equal(t, int64(25), intGaugeValue(t, metrics, "process.memory.rss", "descendants"))
	assert.InDelta(t, 12.5, floatGaugeValue(t, metrics, "process.cpu.usage", "self"), 0.0001)
	assert.InDelta(t, 7.0, floatGaugeValue(t, metrics, "process.cpu.usage", "descendants"), 0.0001)
	assert.Equal(t, int64(2), intGaugeValue(t, metrics, "process.descendants.count", ""))
	assert.Equal(t, int64(1), intGaugeValue(t, metrics, "process.descendants.truncated", ""))

	require.NoError(t, registration.Unregister())
	_ = collectMetrics(t)
	assert.Equal(t, 1, calls)
}

func collectMetrics(t *testing.T) map[string]metricdata.Aggregation {
	t.Helper()
	var resourceMetrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &resourceMetrics))

	metrics := make(map[string]metricdata.Aggregation)
	for _, scope := range resourceMetrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			metrics[metric.Name] = metric.Data
		}
	}
	return metrics
}

func intGaugeValue(t *testing.T, metrics map[string]metricdata.Aggregation, name, scope string) int64 {
	t.Helper()
	gauge, ok := metrics[name].(metricdata.Gauge[int64])
	require.True(t, ok, "%s is not an int64 gauge", name)
	for _, point := range gauge.DataPoints {
		if scope == "" {
			return point.Value
		}
		value, present := point.Attributes.Value(attribute.Key("process.scope"))
		if present && value.AsString() == scope {
			return point.Value
		}
	}
	t.Fatalf("%s has no process.scope=%q data point", name, scope)
	return 0
}

func floatGaugeValue(t *testing.T, metrics map[string]metricdata.Aggregation, name, scope string) float64 {
	t.Helper()
	gauge, ok := metrics[name].(metricdata.Gauge[float64])
	require.True(t, ok, "%s is not a float64 gauge", name)
	for _, point := range gauge.DataPoints {
		value, present := point.Attributes.Value(attribute.Key("process.scope"))
		if present && value.AsString() == scope {
			return point.Value
		}
	}
	t.Fatalf("%s has no process.scope=%q data point", name, scope)
	return 0
}
