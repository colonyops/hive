package drive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/fswatch"
)

// Installed once for the package: the global providers delegate exactly once,
// so the package-level instruments bind to the first ones registered.
var (
	reader *sdkmetric.ManualReader
	spans  *tracetest.SpanRecorder
)

func TestMain(m *testing.M) {
	reader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	spans = tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	os.Exit(m.Run())
}

// operationCount reads one series of drive.operation.duration. Instruments
// are cumulative for the process, so a test asserts on the delta.
func operationCount(t *testing.T, op, actor, result string) uint64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	var total uint64
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if m.Name != "drive.operation.duration" || !ok {
				continue
			}
			for _, dp := range hist.DataPoints {
				gotActor, _ := dp.Attributes.Value("drive.actor")
				gotOp, _ := dp.Attributes.Value("drive.operation")
				gotResult, _ := dp.Attributes.Value("result")
				if gotOp.AsString() == op && gotActor.AsString() == actor && gotResult.AsString() == result {
					total += dp.Count
				}
			}
		}
	}
	return total
}

func watchCount(t *testing.T, outcome string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	var total int64
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if m.Name != "drive.watch.changes" || !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				if got, _ := dp.Attributes.Value("outcome"); got.AsString() == outcome {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func TestOperationsRecordActorAndResult(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	agent := drive.Author{Session: "chat-1"}

	agentWrites := operationCount(t, opWrite, actorAgent, resultOK)
	userRefusals := operationCount(t, opDelete, actorUser, resultRefused)

	_, err := f.svc.Write(ctx, ws, "plans/a.md", agent, []byte("# A\n"))
	require.NoError(t, err)
	err = f.svc.Delete(ctx, ws, "plans/missing.md", drive.Author{})
	require.ErrorIs(t, err, drive.ErrNotFound)

	assert.Equal(t, agentWrites+1, operationCount(t, opWrite, actorAgent, resultOK))
	assert.Equal(t, userRefusals+1, operationCount(t, opDelete, actorUser, resultRefused), "a missing document is a refusal, not a failure")
}

func TestOperationAnnotatesTheCallersSpan(t *testing.T) {
	f := newFixture(t, nil)
	ctx, span := otel.Tracer("test").Start(context.Background(), "mcp.tool write_document")

	_, err := f.svc.Write(ctx, ws, "plans/a.md", drive.Author{}, []byte("# A\n"))
	require.NoError(t, err)
	span.End()

	ended := spans.Ended()
	attrs := ended[len(ended)-1].Attributes()
	assert.Contains(t, attrs, attribute.String(attrSpace, ws.Key))
	assert.Contains(t, attrs, attribute.String(attrPath, "plans/a.md"))
	assert.Contains(t, attrs, attribute.Int(attrBytes, 4))
}

func TestWatchedChangeRootsASpanAndCountsItsOutcome(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	_, err := f.svc.Tree(ctx, acme)
	require.NoError(t, err)
	absRoot, err := filepath.Abs(f.svc.Root())
	require.NoError(t, err)
	w := &watcher{cfgRoot: f.svc.Root(), absRoot: absRoot}
	plans := filepath.Join(absRoot, "acme", "site", "plans")

	published := watchCount(t, watchPublished)
	unchanged := watchCount(t, watchUnchanged)

	f.write(t, acme, "plans/a.md", "# A\n")
	parent, parentSpan := otel.Tracer("test").Start(ctx, "app.startup")
	f.svc.apply(parent, w, fswatch.Change{Dirs: []string{plans}})
	parentSpan.End()
	f.svc.apply(ctx, w, fswatch.Change{Dirs: []string{plans}})

	assert.Equal(t, published+1, watchCount(t, watchPublished))
	assert.Equal(t, unchanged+1, watchCount(t, watchUnchanged), "a second pass over the same folder finds nothing new")

	var changes []sdktrace.ReadOnlySpan
	for _, span := range spans.Ended() {
		if span.Name() == "drive.watch change" {
			changes = append(changes, span)
		}
	}
	require.GreaterOrEqual(t, len(changes), 2)
	change := changes[len(changes)-2]
	assert.False(t, change.Parent().IsValid(), "a change on disk is a trigger, never a child of the span Watch was given")
	assert.Contains(t, change.Attributes(), attribute.Int(attrWatchSpaces, 1))
}
