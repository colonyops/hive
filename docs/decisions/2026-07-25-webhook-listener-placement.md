# The webhook listener stays in the core

- **Status:** accepted
- **Date:** 2026-07-25

## Context

`cmd/desktop/internal/app/sources/webhook.Listener` is a `net/http` server inside `app`, the package
`docs/architecture.md` describes as "No Wails, no transport, no globals", with "No HTTP status codes or
transport concepts in `app/`". Read literally, it looks like a violation waiting to be moved under
`internal/adapter/`.

## Decision

**The webhook listener stays in `cmd/desktop/internal/app/sources/webhook`.** The "no transport" rule
governs driving adapters (`wailsui`, `httpapi`, `mcpsrv`): surfaces that expose `App`'s operations to an
outside caller over a wire. The listener is not that. It is push-mode connector infrastructure, the ingress
half of the `sources.webhook` connector (ADR source-connector-registry), symmetric with the poll producer;
both feed `IngestObservation`.

- The `http.Status*` values `handleHook` writes are the one sanctioned exception to "no transport vocabulary
  in `app/`", scoped to this package. No other package under `app` gets it by analogy.
- Driving adapters that share the loopback server (`httpapi`, `mcpsrv`) mount onto it through
  `App.MountAPI` and stay in their own packages under `cmd/desktop/internal/adapter/`
  (ADR agent-http-api).
- The listener's lifecycle is a connector's: an idempotent, context-taking `Stop`, the template for
  background components in `docs/architecture.md`.

## Consequences

- A future audit must not move the listener into `adapter/`; that would break the connector symmetry.
