package rss

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// reader is installed once for the whole package, as in tmuxcc: the global
// MeterProvider delegates exactly once, so the instruments in metrics.go —
// created at package init — bind to whichever provider is registered first. A
// per-test provider would silently collect nothing.
var reader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	reader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	os.Exit(m.Run())
}

// fetchCount reads the fetch counter for one outcome. Instruments are
// cumulative for the life of the process, so a test asserts on the delta
// around the work it is measuring.
func fetchCount(t *testing.T, result string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))

	var total int64
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "source.rss.fetch" {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, point := range sum.DataPoints {
				if got, present := point.Attributes.Value(attribute.Key("result")); present && got.AsString() == result {
					total += point.Value
				}
			}
		}
	}
	return total
}
