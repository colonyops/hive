# The flow engine moves to Go

- **Status:** accepted
- **Date:** 2026-07-25

## Context

Flow routing between sources and actions has to run without a webview, so that headless surfaces (CLI,
MCP, HTTP, agents) can execute flows and the correctness-critical parts (offset advancement, fan-out,
commit atomicity) sit in one engine. The commit protocol is idempotent-by-offset with unrouted accounting,
and it has load-bearing subtleties: an empty snapshot must stay distinguishable from an ordinary item
event, or the consumer wedges at that offset.

## Decision

The engine is `cmd/desktop/internal/app/runtime`, in Go, in-process. It owns graph indexing, routing,
filter matching, script evaluation, terminal sinks, node-run accounting, and `CommitBatch` construction.

`Runner.Run(ctx, batch) (models.CommitBatch, error)` **does not commit**. It returns what the messages are
worth; reading the log and applying the batch belong to the caller. A dry run and a live tick are the same
code path.

Two registries describe a node type: `flow.registry` says how it is configured, and `runtime.behaviors`
says what it does with a message (process, sink). Sources are relays derived from the connector registry.
Neither the router nor the executor branches on a type string, and a test fails if the registries
disagree.

`runtime/testdata/parity/*.json` fixtures are the engine's regression suite: a flow, a batch of messages,
and the exact expected `CommitBatch`. Only `durMs` (wall-clock) and `err` (runtime wording) are normalized.
A change to routing, sink tagging, or node-run accounting belongs in a fixture.

## Consequences

- `github-filter`'s glob matching is implemented in Go, not delegated to doublestar. The matcher treats
  `[`, `]`, `?`, `{`, `}` as literals (`*[bot]` in particular); doublestar would treat them as pattern
  syntax and silently change routing. `Validate` still uses `doublestar.ValidatePattern` as a save-time
  check.
- Ordering is part of the contract. Every ordered field of a `CommitBatch` must be reproducible, so entry
  nodes are seeded in flow declaration order and outbound ports walk in numeric order. The parity test
  runs each fixture five times and requires identical bytes.
- A flow that cannot execute (a cycle, a node type with no behavior, a function script that does not
  compile) is rejected when the `Runner` is built, at deploy.
- A `Runner` holds per-node state (a function node's `state`) for its lifetime, so a flow pumped page by
  page behaves as one run. It is not safe for concurrent use; the caller serializes.
