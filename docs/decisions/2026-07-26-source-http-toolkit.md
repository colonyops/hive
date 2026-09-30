# A shared HTTP toolkit for source connectors

- **Status:** accepted
- **Date:** 2026-07-26

## Context

ADR owned-github-client moved the GitHub client into the connector that owns it and named
the follow-on work it unblocked: "an injectable transport, middleware, cache
integration." None of it had been built. `ghclient` still hand-rolled every
request — three paths (`getJSONConditional`, `postGraphQL`, `postForm`) each
repeating base-URL joining, `Accept` and version headers, and an
`if c.token != ""` bearer block — and no request was observable at all: the
client held no logger, and `NewProductionClient` had no parameter to give it
one.

GitHub is also about to stop being the only HTTP source. `architecture.md`
names RSS, Grafana, and PostHog as coming pull connectors. Each would need
the same plumbing, and — more importantly — each would arrive with its own
error sentinels. The taxonomy is not decoration: `connect.go` maps
`ErrUnauthorized` onto a disconnected `ConnectionStatus`, and
`feed.LiveProvider` maps `ErrRateLimited` onto a fetch cooldown plus an
activity event. A second provider-local error set means writing both
mappings a second time.

`architecture.md` already required `appkit/httpclient` for new outbound
calls, and recorded adoption in `ghclient` as "a choice now, not a blocker …
whenever composable middleware earns its keep." Observability is what earned
it.

## Decision

**A leaf package, `internal/app/sources/sourcehttp`, holds what any REST
source client needs**, and `ghclient` is built over it.

It carries the failure taxonomy (`ErrUnauthorized`, `ErrRateLimited`,
`ErrUnreachable`, `RateLimitError`), response-status mapping with a
provider-supplied hook for APIs that overload 403, rate-limit header parsing
(`Retry-After`, then `X-RateLimit-Reset`), conditional-request validators
(ETag *and* Last-Modified), and a logging `http.RoundTripper`.

Request construction is **not** reimplemented: `sourcehttp.New` returns an
`appkit/httpclient` client, and base-URL joining, bearer auth, per-call
middleware, and JSON decoding come from there.

Three placement choices are load-bearing:

- **The taxonomy lives here, not in `sources/connector`.** `connector` is the
  natural home for shared vocabulary, but it imports `internal/app/store` and
  so drags the SQLite driver behind it. A client package must not depend on
  the persistence layer to name an error.
- **Logging is a transport, not middleware.** A `RoundTripper` observes the
  request that reaches the wire — after redirects, and regardless of what a
  caller stacks in front. It also needs nothing from appkit: `httpclient.New`
  takes the `*http.Client`, so the two compose with no upstream change.
- **Source semantics stay with the caller.** Response caching, poll cadence,
  and the rate-limit cooldown remain in `feed.LiveProvider`. The cooldown is
  reusable in principle but is currently interleaved with the feed's cache
  and activity recorder; it is left for the second connector to pull up
  rather than shaped from one example.

`ghclient.WithLogger` threads the app logger in, and `NewProductionClient`
— a one-line wrapper whose only logic was an empty-`apiBase` guard — is
deleted in favor of `app.New` calling `ghclient.NewClient` directly.

## Consequences

- Request logging is available for every source at debug level, carrying
  method, URL, status, elapsed time, and `X-RateLimit-Remaining`. That last
  field is the one ADR devserver-github-proxy's proxy exists to conserve, and nothing surfaced
  it before a limit was already hit. Query parameters with secret-looking
  names are redacted before logging.
- Transport failures during a cancelled poll or app shutdown log at debug,
  not error, so a normal quit no longer writes failure lines.
- `Notifications` takes and returns `sourcehttp.Validators` rather than a
  Last-Modified string, so ETag-based conditional polling is available to the
  next connector without reopening the signature.
- `WithTokenCopy` remains a plain struct copy. The token is resolved per
  request through per-call middleware reading the receiver, so clones share
  one connection pool and a clone authenticates as itself. Binding the token
  at client construction instead would have made every clone silently reuse
  the template's token across all six clone sites — the exact multi-account
  isolation the method exists to provide.
- This is an extraction from one implementation. The shape covers what any
  REST API needs; retry, backoff, and pagination are deliberately absent
  until a second connector shows what they should look like.
