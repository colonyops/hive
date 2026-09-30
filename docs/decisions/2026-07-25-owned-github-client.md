# The desktop owns its GitHub client

- **Status:** accepted
- **Date:** 2026-07-25

## Context

Every GitHub request the desktop makes went through the vendored
`internal/hivecore/github.Client` — a client written for the `hive` CLI's
needs, consumed here read-only under the vendoring policy (ADR vendor-hive-internals). An
architecture audit found it had become the largest hole in the hivecore
bounded context: the vendored type appeared in exported constructor
signatures across three `sources/github` files, `ConfirmTerminal` returned
the vendored `Issue` whose fields a second package consumed, and `app.go`
carried a `//nolint:contextcheck` for consuming a vendored signature raw.
An anti-corruption seam (`hive_adapters.go`, narrow local interfaces,
conversion at the boundary) was built to contain it.

The seam contained the types but not the friction. The desktop's fetch
needs had already diverged from the TUI's: per-account response caches,
conditional-request state, and rate-limit cooldowns all live in
`feed.LiveProvider`, layered *around* the client because they could not go
*in* it. `architecture.md`'s own `httpclient` section records the same
wall from another side — the vendored client's transport is a concrete
`*http.Client` field, so composable middleware "cannot reach the one fetch
path that exists" without landing a change upstream and re-vendoring. And
ADR credential-store had already forked the credential half of the integration,
leaving the vendored `token.go` explicitly dead. The client was the
trailing edge of a divergence that had already happened.

The surface at stake is small: ~650 non-test lines (client, device flow,
issue/model types), stable GitHub API territory.

## Decision

**The GitHub client is owned code.** The vendored implementation is ported
faithfully into a leaf package under the connector that owns it
(`internal/app/sources/github/ghclient`), tests included, and the seam is deleted —
there is nothing left to seam. `token.go` is not ported (dead by ADR credential-store).
`internal/hivecore/github` remains in the vendored tree (the sync is
wholesale) but nothing outside `internal/hivecore` imports it.

The port is behavior-identical at the wire: same headers, conditional
requests, rate-limit handling, GraphQL batching, device-flow cadence. What
changes is ownership: context-first signatures throughout, the connector
constructs its own client (the app facade no longer builds a vendored type
to inject), and future desktop-shaped needs — an injectable transport,
middleware, cache integration — are one PR here instead of a cross-repo
round trip.

## Consequences

- The hivecore bounded context shrinks to the seams that earn their keep:
  `dispatch/hive_adapters.go` (sessions, messaging) and the migration
  runner. A depguard rule can now enforce the boundary with a short
  allowlist.
- Upstream fixes to `colonyops/hive`'s GitHub client no longer arrive via
  `mise run vendor`; they must be ported by hand if wanted. Accepted: the
  API surface used is small and slow-moving, and the desktop is the
  primary consumer of this code path.
- `architecture.md`'s stated blocker on adopting `appkit/httpclient` for
  the fetch path is dissolved — the connector now owns its HTTP, which was
  that section's stated adoption trigger.
- ADR vendor-hive-internals's scope narrows in practice: vendoring remains the policy for
  hive's session/messaging/store internals; the GitHub integration is now
  desktop-owned end to end (credentials by ADR credential-store, client by this ADR).
