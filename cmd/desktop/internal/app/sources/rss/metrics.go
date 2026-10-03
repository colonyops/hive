package rss

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/internal/platform/observe"
)

var meter = observe.Meter("/internal/app/sources/rss")

// A counter over a bounded outcome vocabulary. The feed URL is per-user and
// unbounded, so it rides on the span and in the log line, never here.
//
// What it answers: whether conditional polling is working (not_modified should
// dominate a healthy feed), and when it is not, whether the fault is the
// network, the server, or the document.
var fetchCounter = observe.Must(meter.Int64Counter(
	"source.rss.fetch",
	metric.WithDescription("Feed fetches by outcome."),
))

const (
	resultModified     = "modified"
	resultNotModified  = "not_modified"
	resultUnreachable  = "unreachable"
	resultHTTPError    = "http_error"
	resultTooLarge     = "too_large"
	resultParseFailure = "parse_failure"
)

// Built once per outcome: metric.WithAttributes allocates, and this is the
// per-fetch path.
var fetchAttrs = func() map[string]metric.MeasurementOption {
	results := []string{resultModified, resultNotModified, resultUnreachable, resultHTTPError, resultTooLarge, resultParseFailure}
	out := make(map[string]metric.MeasurementOption, len(results))
	for _, result := range results {
		out[result] = metric.WithAttributes(attribute.String("result", result))
	}
	return out
}()
