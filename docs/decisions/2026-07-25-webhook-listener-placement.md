# The webhook listener stays in the core

- **Status:** accepted
- **Date:** 2026-07-25

## Context

`internal/app/sources/webhook.Listener` is a `*net/http.Server` bound to
127.0.0.1, living inside `internal/app` — the package `architecture.md` labels
"THE CORE. No Wails, no transport, no globals." and whose Errors section says
plainly: "No HTTP status codes or transport concepts in `app/`." Read
literally, an `http.Server` and the `http.Status*` constants its handler
returns are exactly what that language excludes. A drive-by audit of the
package looks like a violation waiting to be "fixed" by moving it under
`internal/adapter/`.

It was placed here deliberately, not by drift. ADR local-webhook-listener chose a dedicated
`net/http` server for generic pipeline ingress specifically because push-mode
delivery fits the poll producer badly (a buffer drained per tick would add up
to a full poll interval of latency and distort the producer's "authoritative
snapshot per tick" contract), and wired it as one of the two ingress paths —
`sources.Resolver.PushInstances`, alongside the poll producer's pull-mode
instances — a connector's *way of delivering*, not a different kind of thing
from the producer sitting beside it in the same package tree.

This is being made explicit now, during the lifecycle hardening pass that gave
`Listener.Stop` an idempotent, ctx-taking shutdown (see `App.Close`'s doc
comment and `docs/architecture.md`'s Background lifecycle section), which is a
reasonable point to settle the placement question rather than leave it for
whoever next reads "no transport" literally.

## Decision

**The webhook listener stays in `internal/app/sources/webhook`.** The "no
transport" rule and the Errors section's "no HTTP status codes … in `app/`"
govern *driving* adapters — Wails, a future HTTP API, MCP — surfaces that
expose `App`'s operations to an outside caller over a wire, where transport
vocabulary would otherwise leak into signatures that core code and other
adapters have to consume. The webhook listener is not that: nothing calls into
`App`'s services through it in the RPC sense, and it exposes no operation a
driving adapter would otherwise expose. It is **push-mode connector
infrastructure** — the ingress half of a source connector (ADR source-connector-registry's
registry, `sources.webhook.Descriptor`), symmetric with the poll producer's
ingress half; both feed `IngestObservation` through the same production
boundary. A future `internal/adapter/httpapi` mounting REST/SSE over `App`'s
services is the thing "no transport" actually describes, and it remains a
separate package under `internal/adapter/` when built (`architecture.md`'s
directory structure already reserves that home).

Concretely:

- `http.Server`, `net.Listener`, and the `POST /hooks/<path>` routing stay
  exactly where ADR local-webhook-listener put them.
- The `http.Status*` constants `handleHook` writes (`http.StatusAccepted`,
  `http.StatusUnauthorized`, `http.StatusNotFound`, …) are **the one sanctioned
  exception to "no transport vocabulary in `app/`," scoped to this package**:
  they are HTTP because the ingress mechanism is HTTP, not because a driving
  adapter is answering an RPC. No other package under `internal/app` gets the
  same exception by analogy — this connector's wire responses are a fact about
  it, not a precedent to reuse.
- The listener binds loopback only (127.0.0.1 — ADR local-webhook-listener), never proxies
  `App`'s services, and its lifecycle (`Start`/`Stop`) is a connector's, tested
  and reasoned about the same way the producer's is.

## Consequences

- `depguard`'s `core` rule (forbidding Wails/adapter imports from
  `internal/app`) and the "no transport" language in `architecture.md` are
  read as governing *driving* surfaces, not every use of `net/http`; this ADR
  is the record of that reading, so a future audit does not "fix" the listener
  into `internal/adapter/` and break the connector symmetry ADR local-webhook-listener
  established.
- The future HTTP API adapter (migration step 7) is unaffected: it is a
  genuinely different thing — a driving adapter over `App`'s services — and
  gets its own package under `internal/adapter/httpapi` when built, not a
  merge with this one.
- `Listener.Stop` becoming ctx-taking and idempotent (the same pass that
  prompted this ADR) needed no placement change to land: it is a property of
  the connector's own lifecycle, not evidence for or against where the file
  lives. It also removed the package's only `context.Background()` call, so
  the `forbidigo` exclusion `.golangci.yml` carried for
  `internal/app/sources/webhook/` (a package-wide allowance, only ever needed
  for this one call site) is no longer load-bearing.
