package fswatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Installed once for the package: the global MeterProvider delegates exactly
// once, so the package-level instruments bind to the first one registered.
var reader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	reader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	os.Exit(m.Run())
}

func sum(t *testing.T, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	var total int64
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if data, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == name {
				for _, dp := range data.DataPoints {
					total += dp.Value
				}
			}
		}
	}
	return total
}

// The metric tests are not parallel: every watcher in the process shares the
// instruments, and the parallel tests wait until the sequential ones finish.
func TestDescriptorsAreReleasedOnClose(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a", "b"), 0o755))
	before := sum(t, "fswatch.descriptors")

	w, err := New(zerolog.Nop(), root, WithDebounce(testDebounce))
	require.NoError(t, err)
	assert.Greater(t, sum(t, "fswatch.descriptors"), before)

	require.NoError(t, w.Close())
	assert.Equal(t, before, sum(t, "fswatch.descriptors"))
}

func TestOverflowIsCounted(t *testing.T) {
	root := t.TempDir()
	events := make(chan fsnotify.Event)
	errs := make(chan error)
	w := newWatcher(t, root, withSources(events, errs))
	before := sum(t, "fswatch.overflows")

	errs <- fsnotify.ErrEventOverflow
	nextChange(t, w)

	assert.Equal(t, before+1, sum(t, "fswatch.overflows"))
}
