package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/platform/observe"
)

const (
	// Loki rejects a line over 256 KB. A desktop log message that large is a
	// dump, not a message, so the cap sits well under the ceiling.
	maxBodyBytes = 32 << 10

	maxAttrs          = 64
	maxAttrValueBytes = 2048
)

// logWriter re-emits each encoded zerolog event as an OTLP log record.
//
// It is a writer arm rather than a zerolog.Hook because a Hook is handed only
// the level and the message; an event's fields are not readable from a
// *zerolog.Event. Every arm of a MultiLevelWriter receives the encoded JSON,
// which is what the app's ConsoleWriter arms already parse to pretty-print.
type logWriter struct {
	emit emitter
}

// emitter binds a context to a log emit the way credentials.Bind binds a
// Resolver. An io.Writer has no context parameter, and the bound context is
// detached from app cancellation so shutdown lines are not the ones dropped.
//
// It takes a SpanContext because correlation on an OTLP record comes from the
// context handed to Emit, not from an attribute, and the ids arrive here as
// fields on the encoded event.
type emitter func(trace.SpanContext, otellog.Record)

func bindEmitter(ctx context.Context, logger otellog.Logger) emitter {
	base := context.WithoutCancel(ctx)
	return func(sc trace.SpanContext, rec otellog.Record) {
		if !sc.IsValid() {
			logger.Emit(base, rec)
			return
		}
		logger.Emit(trace.ContextWithSpanContext(base, sc), rec)
	}
}

func newLogWriter(ctx context.Context, logger otellog.Logger) io.Writer {
	return &logWriter{emit: bindEmitter(ctx, logger)}
}

// Write never fails: this is a tap on the log pipeline, and an error here would
// be an error about an error.
func (w *logWriter) Write(p []byte) (int, error) {
	w.forward(p)
	return len(p), nil
}

func (w *logWriter) forward(p []byte) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(p, &fields); err != nil {
		return
	}

	var rec otellog.Record
	rec.SetObservedTimestamp(time.Now())

	if raw, ok := fields[zerolog.TimestampFieldName]; ok {
		if s, err := decodeString(raw); err == nil {
			if ts, err := time.Parse(zerolog.TimeFieldFormat, s); err == nil {
				rec.SetTimestamp(ts)
			}
		}
	}

	level := ""
	if raw, ok := fields[zerolog.LevelFieldName]; ok {
		level, _ = decodeString(raw)
	}
	rec.SetSeverity(severity(level))
	if level != "" {
		rec.SetSeverityText(level)
	}

	body := ""
	if raw, ok := fields[zerolog.MessageFieldName]; ok {
		body, _ = decodeString(raw)
	}
	rec.SetBody(attribute.StringValue(truncate(body, maxBodyBytes)))

	attrs := make([]attribute.KeyValue, 0, len(fields))
	for key, raw := range fields {
		switch key {
		case zerolog.TimestampFieldName, zerolog.LevelFieldName, zerolog.MessageFieldName:
			continue
		// Promoted to the record's own trace context below.
		case observe.LogTraceIDKey, observe.LogSpanIDKey:
			continue
		}
		if len(attrs) == maxAttrs {
			break
		}
		attrs = append(attrs, attribute.KeyValue{Key: attribute.Key(key), Value: attrValue(raw)})
	}
	if len(attrs) > 0 {
		rec.AddAttributes(attrs...)
	}

	w.emit(spanContextFrom(fields), rec)
}

// An unparseable or absent pair yields the zero value, which emits an
// uncorrelated record rather than failing the write.
func spanContextFrom(fields map[string]json.RawMessage) trace.SpanContext {
	traceHex, err := decodeString(fields[observe.LogTraceIDKey])
	if err != nil {
		return trace.SpanContext{}
	}
	spanHex, err := decodeString(fields[observe.LogSpanIDKey])
	if err != nil {
		return trace.SpanContext{}
	}
	traceID, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		return trace.SpanContext{}
	}
	spanID, err := trace.SpanIDFromHex(spanHex)
	if err != nil {
		return trace.SpanContext{}
	}
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
}

func attrValue(raw json.RawMessage) attribute.Value {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return attribute.StringValue(truncate(string(raw), maxAttrValueBytes))
	}
	switch v := decoded.(type) {
	case string:
		return attribute.StringValue(truncate(v, maxAttrValueBytes))
	case bool:
		return attribute.BoolValue(v)
	case float64:
		// zerolog writes integers without a fraction; keep them integral so a
		// backend does not render counts as 3.0.
		if v == float64(int64(v)) {
			return attribute.Int64Value(int64(v))
		}
		return attribute.Float64Value(v)
	case nil:
		return attribute.StringValue("")
	default:
		return attribute.StringValue(truncate(string(raw), maxAttrValueBytes))
	}
}

func decodeString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated " + strconv.Itoa(len(s)-max) + " bytes)"
}

// severity files an unrecognised level as Info rather than leaving it
// unspecified, which sorts unpredictably in a backend.
func severity(level string) otellog.Severity {
	switch level {
	case zerolog.LevelTraceValue:
		return otellog.SeverityTrace
	case zerolog.LevelDebugValue:
		return otellog.SeverityDebug
	case zerolog.LevelInfoValue:
		return otellog.SeverityInfo
	case zerolog.LevelWarnValue:
		return otellog.SeverityWarn
	case zerolog.LevelErrorValue:
		return otellog.SeverityError
	case zerolog.LevelFatalValue:
		return otellog.SeverityFatal
	case zerolog.LevelPanicValue:
		return otellog.SeverityFatal2
	default:
		return otellog.SeverityInfo
	}
}
