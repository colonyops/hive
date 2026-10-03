package procstats

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/internal/platform/observe"
)

var (
	meter = observe.Meter("/internal/app/procstats")

	residentMemory = observe.Must(meter.Int64ObservableGauge(
		"process.memory.rss",
		metric.WithDescription("Resident memory used by the desktop process and its descendants."),
		metric.WithUnit("By"),
	))
	cpuUsage = observe.Must(meter.Float64ObservableGauge(
		"process.cpu.usage",
		metric.WithDescription("CPU used by the desktop process and its descendants, as a percentage of one logical core."),
		metric.WithUnit("%"),
	))
	descendantCount = observe.Must(meter.Int64ObservableGauge(
		"process.descendants.count",
		metric.WithDescription("Descendant processes included in the process resource gauges."),
	))
	descendantsTruncated = observe.Must(meter.Int64ObservableGauge(
		"process.descendants.truncated",
		metric.WithDescription("Whether the descendant process walk reached its safety limit (0 or 1)."),
	))
)

var (
	scopeSelf        = metric.WithAttributes(attribute.String("process.scope", "self"))
	scopeDescendants = metric.WithAttributes(attribute.String("process.scope", "descendants"))
)

type sampleFunc func(context.Context) (Stats, error)

// RegisterMetrics samples this process tree during each metric collection.
// The caller must unregister the returned callback when the sampler is no
// longer in use.
func (s *Sampler) RegisterMetrics() metric.Registration {
	return registerMetrics(s.Sample)
}

func registerMetrics(sample sampleFunc) metric.Registration {
	return observe.Must(meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		stats, err := sample(ctx)
		if err != nil {
			return err
		}

		var descendantsRSS uint64
		var descendantsCPU float64
		for _, child := range stats.Children {
			descendantsRSS += child.RSSBytes
			descendantsCPU += child.CPUPercent
		}

		observer.ObserveInt64(residentMemory, int64(stats.Process.RSSBytes), scopeSelf)
		observer.ObserveInt64(residentMemory, int64(descendantsRSS), scopeDescendants)
		observer.ObserveFloat64(cpuUsage, stats.Process.CPUPercent, scopeSelf)
		observer.ObserveFloat64(cpuUsage, descendantsCPU, scopeDescendants)
		observer.ObserveInt64(descendantCount, int64(len(stats.Children)))
		observer.ObserveInt64(descendantsTruncated, boolInt64(stats.ChildrenTruncated))
		return nil
	}, residentMemory, cpuUsage, descendantCount, descendantsTruncated))
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
