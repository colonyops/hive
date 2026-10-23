# A shared HTTP toolkit for source connectors

- **Status:** accepted
- **Date:** 2026-07-26

## Context

Every pull connector with an HTTP API (GitHub, Gitea, Grafana, PostHog, RSS) needs the same plumbing:
headers, auth, conditional requests, rate-limit parsing, and request logging. Each also needs the same
failure taxonomy, because callers act on it: an unauthorized error maps to a disconnected connection
status, and a rate-limit error maps to a fetch cooldown. A provider-local error set would mean writing
those mappings once per provider.

## Decision

**The leaf package `cmd/desktop/internal/app/sources/sourcehttp` holds what any REST source client needs**,
and the connector API clients are built over it.

It carries the failure taxonomy (`ErrUnauthorized`, `ErrRateLimited`, `ErrUnreachable`,
`RateLimitError`), status mapping with a provider hook for APIs that overload 403
(`ForbiddenClassifier`), rate-limit hint parsing (`Retry-After`, then `X-RateLimit-Reset`), conditional
request validators (ETag and Last-Modified), and a logging `http.RoundTripper`.

Request construction is not reimplemented: `sourcehttp.New` returns an `appkit/httpclient` client, which
provides base-URL joining, auth, per-call middleware, and JSON decoding.

- **The taxonomy lives here, not in `sources/connector`**, so an API client package names its errors
  without importing connector or data-model vocabulary.
- **Logging is a transport, not middleware.** A `RoundTripper` sees the request that reaches the wire,
  after redirects and regardless of caller middleware. `otelhttp` wraps it, so the span covers the same
  request the log line describes.
- **Source semantics stay with the caller.** Response caching, poll cadence, and rate-limit cooldowns live
  in each connector's fetch layer.

## Consequences

- Request logs carry method, URL, status, elapsed time, and `X-RateLimit-Remaining`. Query parameters with
  secret-looking names are redacted.
- Transport failures from a cancelled context log at debug, so a normal quit writes no failure lines.
- `ghclient.WithTokenCopy` is a plain struct copy, and the token is read per request from the receiver.
  Clones share one connection pool and each authenticates as itself. Binding the token at client
  construction would make every clone reuse the template's token and break multi-account isolation.
- Retry, backoff, and pagination are absent on purpose; add them when a connector needs them.
