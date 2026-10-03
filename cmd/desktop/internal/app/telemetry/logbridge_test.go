package telemetry

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type captureExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *captureExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}

func (e *captureExporter) Shutdown(context.Context) error   { return nil }
func (e *captureExporter) ForceFlush(context.Context) error { return nil }

func (e *captureExporter) all() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// newBridge returns a logger whose only writer arm is the bridge.
func newBridge(t *testing.T) (zerolog.Logger, *captureExporter) {
	t.Helper()

	exp := &captureExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.WithoutCancel(t.Context())) })

	w := newLogWriter(t.Context(), lp.Logger(ScopeName))
	return zerolog.New(w).With().Timestamp().Logger().Level(zerolog.TraceLevel), exp
}

func attrs(r sdklog.Record) map[string]attribute.Value {
	out := map[string]attribute.Value{}
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		out[string(kv.Key)] = kv.Value
		return true
	})
	return out
}

// The whole reason the bridge is a writer arm and not a zerolog.Hook: a Hook
// cannot read the event's fields.
func TestBridgeCarriesFields(t *testing.T) {
	logger, exp := newBridge(t)

	logger.Warn().
		Str("path", "/api/terminal").
		Int("count", 3).
		Bool("mounted", true).
		Float64("ratio", 1.5).
		Msg("terminal WebSocket stream mounted")

	records := exp.all()
	require.Len(t, records, 1)
	rec := records[0]

	assert.Equal(t, "terminal WebSocket stream mounted", rec.Body().AsString())
	assert.Equal(t, otellog.SeverityWarn, rec.Severity())
	assert.Equal(t, "warn", rec.SeverityText())
	assert.False(t, rec.Timestamp().IsZero())

	got := attrs(rec)
	assert.Equal(t, "/api/terminal", got["path"].AsString())
	assert.Equal(t, int64(3), got["count"].AsInt64())
	assert.True(t, got["mounted"].AsBool())
	assert.InDelta(t, 1.5, got["ratio"].AsFloat64(), 0.0001)

	// The fields that became record properties must not also be attributes.
	for _, key := range []string{zerolog.LevelFieldName, zerolog.TimestampFieldName, zerolog.MessageFieldName} {
		assert.NotContains(t, got, key)
	}
}

// zerolog writes whole numbers without a fraction; a count must not come out
// the far side as 3.0.
func TestBridgeKeepsIntegersIntegral(t *testing.T) {
	logger, exp := newBridge(t)
	logger.Info().Int64("bytes", 4096).Msg("streamed")

	records := exp.all()
	require.Len(t, records, 1)
	assert.Equal(t, attribute.INT64, attrs(records[0])["bytes"].Type())
}

func TestBridgeMapsSeverity(t *testing.T) {
	tests := []struct {
		level string
		want  otellog.Severity
	}{
		{zerolog.LevelTraceValue, otellog.SeverityTrace},
		{zerolog.LevelDebugValue, otellog.SeverityDebug},
		{zerolog.LevelInfoValue, otellog.SeverityInfo},
		{zerolog.LevelWarnValue, otellog.SeverityWarn},
		{zerolog.LevelErrorValue, otellog.SeverityError},
		{zerolog.LevelFatalValue, otellog.SeverityFatal},
		{zerolog.LevelPanicValue, otellog.SeverityFatal2},
		{"mystery", otellog.SeverityInfo},
		{"", otellog.SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			assert.Equal(t, tt.want, severity(tt.level))
		})
	}
}

// The tap must never fail the write it is observing.
func TestBridgeSwallowsUnparseableWrites(t *testing.T) {
	exp := &captureExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.WithoutCancel(t.Context())) })

	w := newLogWriter(t.Context(), lp.Logger(ScopeName))

	n, err := w.Write([]byte("not json at all"))
	require.NoError(t, err)
	assert.Equal(t, len("not json at all"), n)
	assert.Empty(t, exp.all())
}

func TestBridgeTruncatesOversizedBody(t *testing.T) {
	logger, exp := newBridge(t)
	logger.Info().Msg(strings.Repeat("x", maxBodyBytes+512))

	records := exp.all()
	require.Len(t, records, 1)

	body := records[0].Body().AsString()
	assert.Less(t, len(body), maxBodyBytes+512)
	assert.Contains(t, body, "truncated")
}

func TestBridgeBoundsAttributeCount(t *testing.T) {
	logger, exp := newBridge(t)

	event := logger.Info()
	for i := range maxAttrs + 20 {
		event = event.Int("f"+strconv.Itoa(i), i)
	}
	event.Msg("wide")

	records := exp.all()
	require.Len(t, records, 1)
	assert.Equal(t, maxAttrs, records[0].AttributesLen())
}

func TestBridgeParsesEventTimestamp(t *testing.T) {
	logger, exp := newBridge(t)

	before := time.Now().Add(-time.Second)
	logger.Info().Msg("now")

	records := exp.all()
	require.Len(t, records, 1)
	assert.True(t, records[0].Timestamp().After(before))
	assert.False(t, records[0].ObservedTimestamp().IsZero())
}

// Trace correlation has to reach the record's own trace context, not just its
// attributes: that is the field a backend joins logs to traces on.
func TestBridgeCorrelatesToTheActiveSpan(t *testing.T) {
	logger, exp := newBridge(t)
	logger = logger.Hook(observe.TraceHook)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), sc)

	logger.Info().Ctx(ctx).Str("source", "github").Msg("polled")

	records := exp.all()
	require.Len(t, records, 1)
	assert.Equal(t, sc.TraceID(), records[0].TraceID())
	assert.Equal(t, sc.SpanID(), records[0].SpanID())

	// The ids are the record's own, so repeating them as attributes would put
	// the same value on every correlated line twice.
	got := attrs(records[0])
	assert.NotContains(t, got, observe.LogTraceIDKey)
	assert.NotContains(t, got, observe.LogSpanIDKey)
	assert.Equal(t, "github", got["source"].AsString())
}

// An event with no span is the common case — most of this app's logging is not
// inside a trace — and it must still reach the exporter.
func TestBridgeEmitsUncorrelatedLinesUnchanged(t *testing.T) {
	logger, exp := newBridge(t)
	logger = logger.Hook(observe.TraceHook)

	logger.Info().Msg("no span here")

	records := exp.all()
	require.Len(t, records, 1)
	assert.False(t, records[0].TraceID().IsValid())
	assert.Equal(t, "no span here", records[0].Body().AsString())
}
