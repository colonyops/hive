package mcpsrv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return recorder
}

func TestToolCallRootsASpanNamedForTheTool(t *testing.T) {
	_, session := testSession(t)
	recorder := recordSpans(t)

	call(t, session, "get_status", struct{}{}, nil)
	callErr(t, session, "get_flow", map[string]any{"profileId": "nope"})

	var roots []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if !span.Parent().IsValid() {
			roots = append(roots, span)
		}
	}
	require.Len(t, roots, 2, "a tool call is a trigger, so its span is a root")

	ok, failed := roots[0], roots[1]
	assert.Equal(t, "mcp.tool get_status", ok.Name())
	assert.Equal(t, trace.SpanKindServer, ok.SpanKind())
	assert.Equal(t, codes.Unset, ok.Status().Code)

	assert.Equal(t, "mcp.tool get_flow", failed.Name())
	assert.Equal(t, codes.Error, failed.Status().Code)
	assert.Contains(t, failed.Status().Description, "not_found")
}

// The only test in the package that sets a MeterProvider: the global one
// delegates exactly once, so a second would collect nothing.
func TestToolCallRecordsItsDurationByToolAndOutcome(t *testing.T) {
	_, session := testSession(t)
	reader := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))

	call(t, session, "get_status", struct{}{}, nil)
	callErr(t, session, "get_flow", map[string]any{"profileId": "nope"})

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	counts := map[string]uint64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if m.Name != "mcp.server.operation.duration" || !ok {
				continue
			}
			for _, dp := range hist.DataPoints {
				tool, _ := dp.Attributes.Value("gen_ai.tool.name")
				errType, _ := dp.Attributes.Value("error.type")
				counts[tool.AsString()+"|"+errType.AsString()] += dp.Count
			}
		}
	}
	assert.Equal(t, map[string]uint64{"get_status|": 1, "get_flow|tool_error": 1}, counts)
}
