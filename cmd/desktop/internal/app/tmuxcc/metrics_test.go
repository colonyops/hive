//go:build !server

package tmuxcc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// reader is installed once for the whole package. The global MeterProvider
// delegates exactly once, so the instruments in metrics.go — created at package
// init — bind to whichever provider is registered first and stay bound to it.
// A per-test provider would silently collect nothing.
var reader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	reader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	os.Exit(m.Run())
}

// Every instrument is cumulative for the life of the process, so a test reads a
// value before and after the work it is measuring and asserts on the delta.

func counterSum(t *testing.T, name string) int64 {
	t.Helper()
	data, ok := metricData(t, name).(metricdata.Sum[int64])
	if !ok {
		return 0
	}
	var total int64
	for _, dp := range data.DataPoints {
		total += dp.Value
	}
	return total
}

func counterSumWhere(t *testing.T, name, key, value string) int64 {
	t.Helper()
	data, ok := metricData(t, name).(metricdata.Sum[int64])
	if !ok {
		return 0
	}
	var total int64
	for _, dp := range data.DataPoints {
		if got, present := dp.Attributes.Value(attribute.Key(key)); present && got.AsString() == value {
			total += dp.Value
		}
	}
	return total
}

func histogramCount(t *testing.T, name string) uint64 {
	t.Helper()
	data, ok := metricData(t, name).(metricdata.Histogram[int64])
	if !ok {
		return 0
	}
	var total uint64
	for _, dp := range data.DataPoints {
		total += dp.Count
	}
	return total
}

// streamCounts is every data-plane instrument read at one instant.
type streamCounts struct {
	bytes   int64
	paused  int64
	resumed int64
	depth   uint64
}

func readStreamCounts(t *testing.T) streamCounts {
	t.Helper()
	return streamCounts{
		bytes:   counterSum(t, "tmux.stream.bytes"),
		paused:  counterSumWhere(t, "tmux.stream.lifecycle", "state", "paused"),
		resumed: counterSumWhere(t, "tmux.stream.lifecycle", "state", "resumed"),
		depth:   histogramCount(t, "tmux.stream.buffer.depth"),
	}
}

func metricData(t *testing.T, name string) metricdata.Aggregation {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m.Data
			}
		}
	}
	return nil
}
