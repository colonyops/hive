package sourcehttp

import (
	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/internal/platform/observe"
)

var meter = observe.Meter("/internal/app/sources/sourcehttp")

// A gauge, not a counter: a provider's window resets, so the last reading is
// the answer and a rate over it means nothing.
var rateLimitGauge = observe.Must(meter.Int64Gauge(
	"source.ratelimit.remaining",
	metric.WithDescription("Requests left in the provider's rate-limit window, as of the last response that reported one."),
))
