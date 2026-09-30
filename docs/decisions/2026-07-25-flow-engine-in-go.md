# The flow engine moves to Go

- **Status:** accepted
- **Date:** 2026-07-25

## Context

Execution was split down the middle. Go polled sources and appended to the event log, Go executed actions, and the **browser** did everything in between: topological order, port routing, filter matching, function-node evaluation, terminal sink tagging, and construction of the `CommitBatch` that made all of it durable. The Go side had no way to produce a `CommitBatch` at all.

This was deliberate and documented rather than drift, but it caps the roadmap. A headless process can poll and can execute, and cannot route anything between them — so a CLI, an MCP tool, an HTTP surface and an embedded agent are all impossible for anything involving a flow, and the desktop window is a hard dependency of flow execution. It also puts the correctness-critical parts (offset advancement, fan-out, commit atomicity) out of reach of any surface that is not a webview.

The port is not large. Only two node types have a browser-side runtime: `github-filter`, whose rule is about twenty lines, and `function`. The three terminals contribute a sink-tagging function each. Go already parses and validates every node type through `flow`'s registry. What makes it risky is subtlety rather than volume — the commit protocol is idempotent-by-offset with unrouted accounting, and it has at least one load-bearing quirk (an empty snapshot must stay distinguishable from an ordinary item event, or the consumer wedges at that offset forever).

## Decision

The engine is `internal/app/runtime`, in Go, in-process. It owns graph indexing, routing, filter matching, script evaluation, terminal sinks, node-run accounting, and `CommitBatch` construction.

`Runner.Run(ctx, batch) (store.CommitBatch, error)` **does not commit**. It takes the messages it is given and returns what they are worth; reading the log and applying the batch belong to the caller. That is what makes a dry-run and a live tick the same code path rather than two implementations of the same semantics — the difference is whether the result is handed to `store.CommitBatch`.

Two registries describe a node type between them: `flow.registry` says how it is configured, and `runtime.behaviors` says what it does when a message arrives — relay (sources), sink (terminals), or process (filter, function). Neither the router nor the executor branches on a type string, and a test fails if a type appears in one registry and not the other.

**Correctness is established by shared fixtures, not by review.** `internal/app/runtime/testdata/parity/*.json` are executed by *both* engines: a Go test runs them through `Runner`, and a vitest spec reads the same files and runs them through `runGraph` with the in-process transport. Each compares against the same expected commit. Only two fields are normalized away — `durMs`, which is wall-clock, and `err`, which is each engine's own wording for a thrown value. The fixtures were chosen for the subtleties rather than for coverage of node types: unrouted accounting, snapshot routing including the empty snapshot, the rule that a snapshot may never reach a side-effecting terminal, fan-out isolation, unwired-port discards, port-indexed returns, disabled nodes, node errors, and an offset that is not the last in its batch.

## Consequences

- Both engines exist for one step. That is the point: while both exist the fixtures prove they agree, and a fixture only one of them satisfies means the port is not finished. The frontend engine is deleted in the next step, and the fixtures stay as the Go engine's own regression suite.
- Roughly 1,100 lines of TypeScript become deletable — `engine/`, `driver.ts`, `processors.ts`, the `nodes/*/runtime.ts` files, and the runtime pump — along with the replay orchestration in `useFlowsSession.ts`. Six `PipelineService` methods stop being RPCs and become internal calls.
- `github-filter`'s glob matching is **reimplemented in Go rather than delegated to doublestar**. The shipped matcher treats `[`, `]`, `?`, `{` and `}` as literals; doublestar treats them as character classes, wildcards and alternation. Handing patterns straight to doublestar would silently change which items a deployed flow routes — `*[bot]` in particular. `Validate` keeps `doublestar.ValidatePattern`, which is a save-time check on what a user typed, not the matching rule.
- Ordering is part of the contract now. Every ordered field of a `CommitBatch` — outputs, discards, node runs, feed snapshots — has to be reproducible, so entry nodes are seeded in flow declaration order and outbound ports are walked in numeric order. A test re-runs every fixture five times and requires identical bytes, because Go's map iteration would otherwise make this fail intermittently rather than immediately.
- A flow that cannot be executed is rejected when the `Runner` is built rather than when a message arrives: a cycle, a node type with no runtime behaviour, and a function node whose script does not compile all fail at deploy, where a user is looking.
- The engine holds per-node state (a function node's `state` object) for the life of a `Runner`, so a flow pumped page by page behaves as one continuous run. A `Runner` is therefore not safe for concurrent use — one flow is one consumer of one ordered log, and the caller serialises.
