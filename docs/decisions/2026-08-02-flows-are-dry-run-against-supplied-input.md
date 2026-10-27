# Flows are dry-run against supplied input, not deployed to be observed

- **Status:** accepted
- **Date:** 2026-08-02

## Context

Answering "what does this node output for this input?" required deploying an
edit, waiting for a source poll, and inferring behaviour from the inbox. That
writes feed membership, inbox rows, notifications and node KV to answer a
question about a script, and cannot tell a bug from a source that has not
polled. Per-entity fan-out (ADR function-node-per-entity-feed-items) could not
be checked any other way (#166).

The engine (`cmd/desktop/internal/app/runtime`) never writes the database,
reads durable KV through a port, and returns a batch for its caller to commit.
What was missing was a way to drive it with injected input and see per-node
results.

## Decision

**The `execute_flow` MCP tool runs a flow against caller-supplied input and
reports what every node did, committing nothing** (`FlowsService.DryRun`,
`runtime.Runner.DryRun`).

- **Input is injected at a node id, not routed by topic.** That lets one
  `function` node be exercised against a captured payload in isolation. A
  source node is a relay at run time, so injecting at one is the same as that
  source returning those messages, and nothing fetches.
- **The flow may be a document not on disk.** `flowId` runs an installed flow;
  `flow` or `flowYaml` run a document through the same migrate-then-decode path
  a file takes, so an unsaved edit can be tested.
- **`kv` is an in-memory sandbox seeded by the request.** Durable KV is neither
  read nor written; what the run would store comes back as mutations. Dedup and
  notify-once need a known starting state, and seeding makes calls repeatable.
- **A dry run gets its own `Runner`, always**, closed after the call. A runner
  carries each function node's `state` between messages, so sharing the live
  one would mutate live state.

The result is per node: what it received, what it emitted per output port, its
drops, timing, console output, and a structured error with line and column. An
emission is recorded before the wire check, so an unwired port still reports
what the node put on it.

**`console` is bound in the script VM.** It writes to a per-invocation sink; a
live run passes none, so the calls go nowhere.

## Consequences

- The tool executes author-supplied JavaScript. That is no new exposure:
  anything that reaches it can already write a flow file into the watched
  config directory, and function nodes are author-trusted (ADR
  goja-script-runtime). Dry runs share the process-wide script pool, so a
  wedged script draws on the same bounded budget as a live one.
- Parse and build failures reach the caller verbatim, because the diagnostic is
  the answer.
- Two commit-time effects are absent: minting an inbox row for a key no source
  ingested, and snapshot reconciliation's feed removals. The outputs and feed
  snapshots that would drive them are reported instead.
- The console trace is reachable from more than one goroutine (a script that
  outlives its interrupt can still log), so it takes a lock and caps total
  lines.
