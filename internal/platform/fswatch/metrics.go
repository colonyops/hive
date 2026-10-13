package fswatch

import (
	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/internal/platform/observe"
)

// No instrument is labelled by root: a watched root is a per-user path.
var (
	meter = observe.Meter("/internal/platform/fswatch")

	// On kqueue a watched directory holds one descriptor per entry, so this is
	// what runs a process out of its open-file limit.
	heldDescriptors = observe.Must(meter.Int64UpDownCounter(
		"fswatch.descriptors",
		metric.WithDescription("Descriptors the file watchers hold, by their own count."),
		metric.WithUnit("{descriptor}"),
	))

	overflows = observe.Must(meter.Int64Counter(
		"fswatch.overflows",
		metric.WithDescription("Kernel event queue overflows, each of which forced a resync of every watched directory."),
	))

	budgetExhausted = observe.Must(meter.Int64Counter(
		"fswatch.budget.exhausted",
		metric.WithDescription("Watchers that reached their descriptor budget and left directories unwatched."),
	))
)
