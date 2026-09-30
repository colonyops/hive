// Package sourcehttp is the HTTP toolkit a pull connector's API client is
// built over: the failure taxonomy, status and rate-limit mapping,
// conditional requests, and a logging transport. Provider vocabulary and
// source semantics (caching, poll cadence, cooldowns) stay with the caller.
package sourcehttp

import (
	"net/http"
	"time"

	"github.com/hay-kot/appkit/httpclient"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// server.address cannot name the connector: one host serves several, and the
// dev proxy serves them all.
const attrSource = "source"

const DefaultTimeout = 30 * time.Second

type Config struct {
	// Name identifies the provider ("github") in log lines and error messages.
	Name    string
	BaseURL string
	Logger  zerolog.Logger
	// Token is resolved per request, so rotation needs no rebuild. nil is
	// anonymous.
	Token   func() string
	Timeout time.Duration
	// Transport is the base the logging transport wraps; nil uses
	// http.DefaultTransport.
	Transport http.RoundTripper
}

// New builds the client a source's API calls go through. Logging is installed
// as a transport rather than middleware so it observes the request that
// reaches the wire, after redirects and regardless of caller middleware.
//
// otelhttp wraps the logging transport, so the span covers everything the log
// line describes. Its semconv metrics are the whole of this package's request
// instrumentation; a hand-written counter would duplicate them under a name no
// dashboard knows.
func New(cfg Config, mws ...httpclient.Middleware) *httpclient.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: otelhttp.NewTransport(
			NewTransport(cfg.Name, cfg.Logger, cfg.Transport),
			otelhttp.WithSpanNameFormatter(spanName(cfg.Name)),
		),
	}

	chain := make([]httpclient.Middleware, 0, len(mws)+1)
	if cfg.Token != nil {
		chain = append(chain, httpclient.BearerAuth(cfg.Token))
	}
	chain = append(chain, mws...)

	return httpclient.New(httpClient, cfg.BaseURL, chain...)
}

// Provider and method, never the path: a source path carries org and repository
// names. The "http." prefix says which layer produced the span.
func spanName(source string) func(string, *http.Request) string {
	return func(_ string, r *http.Request) string { return "http." + source + " " + r.Method }
}
