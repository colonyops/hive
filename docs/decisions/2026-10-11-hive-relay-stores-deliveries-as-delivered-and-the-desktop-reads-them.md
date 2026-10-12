# Hive Relay stores deliveries as delivered and the desktop reads them

- **Status:** accepted
- **Date:** 2026-10-11

## Context

Hive Desktop's only push path is the local webhook listener, and it binds
loopback on purpose (ADRs local-webhook-listener, webhook-listener-placement).
The senders people most want in an inbox cannot reach it: CI jobs, servers,
uptime monitors, phones. A tunnel only works while the laptop is awake, and an
alert tends to arrive while it is not (#519).

A hosted endpoint for those senders is public by necessity, so whatever it
runs on a request body is attack surface. The desktop already owns flows,
identity, the canonical item contract, and the credential store, and it can
pull on its poll tick.

## Decision

- **Hive Relay is its own program, `cmd/relay` (`hive-relay`), and it parses
  nothing.** Any request on a source's ingress URL is stored as delivered:
  method, path, query, headers with credentials redacted, and the body as
  bytes. The desktop pulls deliveries by a UUIDv7 cursor.
- **Every reading of a delivery lives in the desktop's `sources.relay`
  connector**: the dialects, identity keys, the field map, and the
  healthchecks clock. A new sender format is a desktop change, never a relay
  change.
- **The relay's work is auth, caps, and rate limits.** Every guard runs before
  the body is read.
- **Sign-in is an emailed code or a configured password, behind a glob
  allowlist, plus API keys. No OAuth.** The lists are checked on every
  request, so removing an entry revokes that email's tokens.
- **Multi-user from the start.** An account is an email, and every store
  operation is scoped to the account the bearer token resolves to. The
  operator is not a user: they act through host subcommands that open the
  store directly, and there is no admin role over HTTP.
- **The store is a port**, `relay.Store`, with one `sqlite` driver; tests run
  it in memory. The access pattern stays an append-only log per source plus
  keyed records, so an object-store driver stays possible.
- **An account can turn on age sealing** to its desktops' recipients. It is
  off by default.
- **The HTTP plumbing is shared.** The relay and the desktop both build on
  `internal/web` (ADR http-handler-conventions).

## Consequences

- The public endpoint has no parser, template, or script to exploit.
- The desktop carries every parser and must ship one before a sender format
  works. A relay deployment never needs an upgrade for a new format.
- `internal/web` is shared plumbing outside the engine's layers, and the
  `web-is-plumbing` depguard rule keeps it off the engine, the stores, and
  every program. `relay-no-program-deps` keeps the relay off the desktop and
  the CLI.
- The relay ships in the shared version (ADR
  every-program-ships-under-one-shared-version): its own release notes under
  `cmd/relay/releasenotes/`, and an image, `ghcr.io/colonyops/hive-relay`,
  tagged with the version and `latest`.
- Bodies are plaintext at rest unless the account turns sealing on.
