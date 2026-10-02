package activity

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// The global MeterProvider delegates exactly once, so the instrument in
// metrics.go binds to the first provider registered. One reader serves the
// package; a per-test provider would collect nothing.
var reader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	reader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	os.Exit(m.Run())
}

type fakeRecorder struct{ events []Event }

func (f *fakeRecorder) Record(_ context.Context, e Event) { f.events = append(f.events, e) }

// The counter is cumulative for the life of the process, so a test reads
// before and after and asserts on the difference. The tests share the
// instrument and must not run in parallel.
func readCounts(t *testing.T) map[attrKey]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	out := map[attrKey]int64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "activity.events" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range sum.DataPoints {
				category, _ := dp.Attributes.Value("category")
				severity, _ := dp.Attributes.Value("severity")
				out[attrKey{Category(category.AsString()), Severity(severity.AsString())}] += dp.Value
			}
		}
	}
	return out
}

func since(t *testing.T, before map[attrKey]int64) map[attrKey]int64 {
	t.Helper()
	out := map[attrKey]int64{}
	for key, value := range readCounts(t) {
		if delta := value - before[key]; delta != 0 {
			out[key] = delta
		}
	}
	return out
}

func TestMetered_HasALabelSetForEveryPair(t *testing.T) {
	m := Metered(&fakeRecorder{})
	before := readCounts(t)

	want := map[attrKey]int64{}
	for _, category := range CategoryNames() {
		for _, severity := range SeverityNames() {
			m.Record(t.Context(), Event{Title: "x", Category: Category(category), Severity: Severity(severity)})
			want[attrKey{Category(category), Severity(severity)}] = 1
		}
	}

	assert.Equal(t, want, since(t, before))
}

func TestMetered_CountsAnEmptyCategoryOrSeverityUnderTheDefault(t *testing.T) {
	m := Metered(&fakeRecorder{})
	before := readCounts(t)

	m.Record(t.Context(), Event{Title: "something happened"})

	assert.Equal(t, map[attrKey]int64{{CategorySystem, SeverityInfo}: 1}, since(t, before))
}

func TestMetered_DoesNotCountAnUnknownCategoryOrSeverity(t *testing.T) {
	inner := &fakeRecorder{}
	m := Metered(inner)
	before := readCounts(t)

	badCategory := Event{Title: "x", Category: Category("nope"), Severity: SeverityInfo}
	badSeverity := Event{Title: "x", Category: CategorySystem, Severity: Severity("loud")}
	m.Record(t.Context(), badCategory)
	m.Record(t.Context(), badSeverity)

	assert.Empty(t, since(t, before))
	assert.Equal(t, []Event{badCategory, badSeverity}, inner.events, "the decorator forwards the event unchanged; rejecting it is the real recorder's job")
}
