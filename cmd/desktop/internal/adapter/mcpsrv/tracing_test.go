package mcpsrv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
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
