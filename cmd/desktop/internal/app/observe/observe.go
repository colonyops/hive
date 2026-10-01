// Package observe is the OpenTelemetry API surface the rest of the app calls.
// It holds no SDK and abstracts nothing: Tracer and Meter return the real API
// types, resolved against the global provider, which is the no-op provider
// until cmd/desktop/internal/app/telemetry registers a real one. Only the scope-name
// convention is centralized here
// (https://opentelemetry.io/blog/2026/dont-wrap-opentelemetry/).
package observe

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Prefixes every scope name, so a backend groups this app's instrumentation
// apart from a library's.
const modulePath = "github.com/colonyops/hive/cmd/desktop"

// Tracer names a package by its path relative to the program root
// ("/internal/app/store"). Assign it to a package-level var.
func Tracer(pkgpath string) trace.Tracer { return otel.Tracer(modulePath + pkgpath) }

func Meter(pkgpath string) metric.Meter { return otel.Meter(modulePath + pkgpath) }

// Must unwraps an instrument constructor. Its arguments are compile-time
// constants, so an error is a programming mistake the first run catches.
func Must[T any](instrument T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("observe: instrument: %v", err))
	}
	return instrument
}

// RecordError marks a span failed. Without the status a backend renders the
// span as successful with an error event hanging off it.
func RecordError(span trace.Span, err error, msg ...string) {
	span.RecordError(err)
	if len(msg) > 0 {
		span.SetStatus(codes.Error, msg[0])
		return
	}
	span.SetStatus(codes.Error, err.Error())
}

// End ends span, failing it when err points at one. It pairs with a named
// return, `defer observe.End(span, &err)`, for the common case where the
// error a function returns is the thing the span exists to explain.
//
// A pointer because defer evaluates its arguments immediately: an error value
// would be read before the function had one.
func End(span trace.Span, err *error) {
	if *err != nil {
		RecordError(span, *err)
	}
	span.End()
}

// StartConditionalSpan starts a span only when ctx already carries one. A wait
// is worth recording under the work that caused it and worthless as a root of
// its own (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
func StartConditionalSpan(ctx context.Context, tracer trace.Tracer, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return ctx, trace.SpanFromContext(ctx)
	}
	return tracer.Start(ctx, name, opts...)
}

// Named to match Grafana's default Loki-to-Tempo derived field.
const (
	LogTraceIDKey = "trace_id"
	LogSpanIDKey  = "span_id"
)

// TraceHook writes the active span's ids onto a log event. It fires only where
// a call site wrote .Ctx(ctx); it never guesses.
var TraceHook zerolog.Hook = traceHook{}

type traceHook struct{}

func (traceHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	sc := trace.SpanContextFromContext(e.GetCtx())
	if !sc.IsValid() {
		return
	}
	e.Str(LogTraceIDKey, sc.TraceID().String())
	e.Str(LogSpanIDKey, sc.SpanID().String())
}
