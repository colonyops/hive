package sourcehttp

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// sensitiveParams are query keys whose values are redacted before logging.
var sensitiveParams = map[string]bool{
	"access_token": true,
	"api_key":      true,
	"key":          true,
	"password":     true,
	"secret":       true,
	"token":        true,
}

type transport struct {
	name string
	log  zerolog.Logger
	next http.RoundTripper
	// Built once: metric.WithAttributes allocates and this is a per-request path.
	attrs metric.MeasurementOption
}

func NewTransport(name string, log zerolog.Logger, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return transport{
		name:  name,
		log:   log,
		next:  next,
		attrs: metric.WithAttributes(attribute.String(attrSource, name)),
	}
}

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	target := redactQuery(r.URL)

	// otelhttp wraps this transport, so the Labeler is already on the context and
	// is read back after this returns.
	if labeler, ok := otelhttp.LabelerFromContext(ctx); ok {
		labeler.Add(attribute.String(attrSource, t.name))
	}

	t.log.Debug().
		Ctx(ctx).
		Str("source", t.name).
		Str("method", r.Method).
		Str("url", target).
		Msg("request")

	start := time.Now()
	resp, err := t.next.RoundTrip(r)
	elapsed := time.Since(start)

	if err != nil {
		// A cancelled poll or a quit during a device-flow wait is routine, not
		// a failure worth an error line.
		level := zerolog.ErrorLevel
		if ctx.Err() != nil {
			level = zerolog.DebugLevel
		}
		t.log.WithLevel(level).
			Ctx(ctx).
			Str("source", t.name).
			Str("method", r.Method).
			Str("url", target).
			Dur("elapsed", elapsed).
			Err(err).
			Msg("request failed")
		return nil, err
	}

	event := t.log.Debug().
		Ctx(ctx).
		Str("source", t.name).
		Str("method", r.Method).
		Str("url", target).
		Int("status", resp.StatusCode).
		Dur("elapsed", elapsed)
	if remaining, ok := rateLimitRemaining(resp.Header); ok {
		event = event.Int("ratelimit_remaining", remaining)
		rateLimitGauge.Record(ctx, int64(remaining), t.attrs)
	}
	event.Msg("response")

	return resp, nil
}

func rateLimitRemaining(h http.Header) (int, bool) {
	value := h.Get("X-RateLimit-Remaining")
	if value == "" {
		return 0, false
	}
	remaining, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return remaining, true
}

func redactQuery(u *url.URL) string {
	if u.RawQuery == "" {
		return u.String()
	}
	params, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return u.Scheme + "://" + u.Host + u.Path + "?<unparsable>"
	}
	redacted := false
	for key := range params {
		if sensitiveParams[key] {
			params.Set(key, "REDACTED")
			redacted = true
		}
	}
	if !redacted {
		return u.String()
	}
	clone := *u
	clone.RawQuery = params.Encode()
	return clone.String()
}
