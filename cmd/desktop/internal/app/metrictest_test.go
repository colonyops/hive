package app

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// The global MeterProvider delegates exactly once, so an instrument a
// dependency created at package init binds to the first provider registered.
// One reader serves the package; a per-test provider would collect nothing.
var meterReader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	meterReader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(meterReader)))
	os.Exit(m.Run())
}

// counterValue sums the data points of one counter that carry every attribute
// in attrs. Counters are cumulative for the life of the process, and parallel
// tests may add to the same series, so a test asserts a floor on the delta
// around the work it measures.
func counterValue(t *testing.T, name string, attrs ...attribute.KeyValue) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, meterReader.Collect(t.Context(), &rm))
	var total int64
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range sum.DataPoints {
				if hasAttributes(dp.Attributes, attrs) {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func hasAttributes(set attribute.Set, attrs []attribute.KeyValue) bool {
	for _, want := range attrs {
		got, ok := set.Value(want.Key)
		if !ok || got.String() != want.Value.String() {
			return false
		}
	}
	return true
}
