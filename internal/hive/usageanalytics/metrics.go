package usageanalytics

import (
	"github.com/colonyops/hive/internal/platform/observe"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	meter              = observe.Meter("/internal/hive/usageanalytics")
	localSinkAttribute = metric.WithAttributes(attribute.String("sink", "local"))
	acceptedEvents     = observe.Must(meter.Int64Counter("usageanalytics.events.accepted"))
	droppedEvents      = observe.Must(meter.Int64Counter("usageanalytics.events.dropped"))
	failedBatches      = observe.Must(meter.Int64Counter("usageanalytics.batches.failed"))
	batchSize          = observe.Must(meter.Int64Histogram("usageanalytics.batch.size", metric.WithExplicitBucketBoundaries(1, 8, 16, 32)))
	writeDuration      = observe.Must(meter.Float64Histogram("usageanalytics.write.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5, 1, 2)))
)
