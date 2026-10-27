# The desktop owns its GitHub client

- **Status:** accepted
- **Date:** 2026-07-25

## Context

The desktop's GitHub fetch needs (per-account response caches, conditional-request state, rate-limit
cooldowns, a composable transport) diverged from the hive CLI's. A client shaped for the CLI forced that
behavior to be layered around it instead of built in, and its concrete `*http.Client` field kept
middleware out of the one fetch path.

## Decision

**The GitHub client is desktop-owned code**, a leaf package under the connector that uses it:
`cmd/desktop/internal/app/sources/github/ghclient` (REST and GraphQL client, device flow, models, item
state). It is built on `appkit/httpclient`, so headers, transport, and middleware compose there. The
credential half is `internal/platform/credentials` (ADR credential-store).

## Consequences

- The hive CLI does not share this client; its issue and PR sources shell out to `gh` and `tea`
  (`cmd/hive/internal/sources`).
- Desktop-shaped needs (transport, middleware, cache integration) change `ghclient` directly.
