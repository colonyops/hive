package activity

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
)

var (
	meter = observe.Meter("/internal/app/activity")

	// Category and Severity are enums, so the label set is bounded. The title,
	// body, and source are per-user and unbounded: they stay on the row.
	recorded = observe.Must(meter.Int64Counter(
		"activity.events",
		metric.WithDescription("Activity log events recorded, by category and severity."),
	))
)

type attrKey struct {
	category Category
	severity Severity
}

// Built once per pair: metric.WithAttributes allocates.
var eventAttrs = func() map[attrKey]metric.MeasurementOption {
	categories, severities := CategoryNames(), SeverityNames()
	out := make(map[attrKey]metric.MeasurementOption, len(categories)*len(severities))
	for _, category := range categories {
		for _, severity := range severities {
			out[attrKey{Category(category), Severity(severity)}] = metric.WithAttributes(
				attribute.String("category", category),
				attribute.String("severity", severity),
			)
		}
	}
	return out
}()

// Metered counts every event recorded through inner, whether or not inner
// persisted it: the interface reports no outcome. An empty category or
// severity counts under the system or info default the recorder fills in. An
// unknown one is not counted: the recorder rejects it, and the label set is
// the enum.
func Metered(inner Recorder) Recorder {
	return metered{inner: inner}
}

type metered struct{ inner Recorder }

func (m metered) Record(ctx context.Context, e Event) {
	m.inner.Record(ctx, e)
	key := attrKey{category: e.Category, severity: e.Severity}
	if key.category == "" {
		key.category = CategorySystem
	}
	if key.severity == "" {
		key.severity = SeverityInfo
	}
	if attrs, ok := eventAttrs[key]; ok {
		recorded.Add(ctx, 1, attrs)
	}
}
