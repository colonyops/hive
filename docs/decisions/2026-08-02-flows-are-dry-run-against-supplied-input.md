# Flows are dry-run against supplied input, not deployed to be observed

- **Status:** accepted
- **Date:** 2026-08-02

## Context

Answering "what does this node output for this input?" required deploying. An
author — or an agent — edited the flow YAML, waited for a real source poll (or
forced `POST /api/sources/refresh`), then inferred the node's behaviour from
what appeared in the inbox. That loop writes feed membership, inbox rows,
notifications and node KV to answer a question about a script, and it cannot
distinguish a bug from a source that has not polled yet.

ADR function-node-per-entity-feed-items's per-entity fan-out is the case that made this untenable. Neither the
array return nor the empty-array reconciliation could be checked without
deploying to the live profile and waiting for a metrics poll, and there was no
way to hand the node a captured payload at all. Issue #166.

The engine was already built for this: `internal/app/runtime` never writes the
database, reads durable KV only through a port, and returns a `CommitBatch` for
its caller to apply. What was missing was a way to *drive* it with input that
did not come from the log, and a way to see what happened beyond the aggregate
counters a `NodeRunView` carries.

## Decision

**`POST /api/flows/execute` executes a flow against caller-supplied input and
reports what every node did, committing nothing.** Four choices define it:

**Input is injected at a node id, not routed by topic.** A live run offers each
message to the entry nodes whose topic it matches. A dry run delivers to the
node the caller names, whatever it is. That is what lets one `function` node be
exercised against a captured payload with the source in front of it never
running — the isolation the deploy loop cannot express. A source node is a relay
at run time (the core ingests for it), so injecting at one is indistinguishable
from that source having polled and returned exactly these messages, and nothing
fetches.

**The flow may be a document that is not on disk.** `flowId` runs an installed
flow; `flow` / `flowYaml` run a document through the same migrate-then-decode
path a file goes through, under a throwaway id. Testing an unsaved edit is what
makes this a testing tool rather than "run the installed flow harder".

**`kv` is an in-memory sandbox seeded by the request.** Durable node KV is
neither read nor written; what the run would have stored comes back as
mutations. Dedup, notify-once and notify-on-change are the node behaviours
hardest to reason about and the ones a dry run most needs to exercise, and
either alternative — no `kv` at all, or the real one — gives a misleading answer
for exactly those. Seeding also makes a call repeatable, which is what an agent
debugging in a loop depends on.

**A dry run gets its own `Runner`, always.** A `Runner` carries each function
node's `state` object between messages, so routing a dry run through the
engine's installed runner would mutate the state a live run depends on and make
consecutive identical calls differ. The runner is built for the call and closed
after it.

The result is per-node rather than aggregate: what each node received, what it
emitted **per output port**, its drops, its timing, its console output, and a
structured error with line and column for a script failure. An emission is
recorded before the wire check, so a port with nothing wired to it still
reports what the node put on it — otherwise "what does this node output" would
depend on the graph downstream, which is the question it is asked instead of.

**`console` is now bound in the script VM.** It had never existed, so a
`console.log` left in an author's script threw `ReferenceError` and dropped the
message. It is bound unconditionally and writes to a per-invocation sink; a live
run passes none, so the calls go nowhere and the hot path is unchanged.

## Consequences

- The endpoint is unauthenticated behind the loopback bind, like the rest of the
  surface (ADR agent-http-api), and it executes author-supplied JavaScript. That is not a
  new exposure: anything that can reach the API can already write a flow file
  through the config directory the same process watches, and function nodes are
  author-trusted by design (ADR goja-script-runtime). Script evaluation shares the process-wide
  `ScriptPool`, so a wedged dry-run script consumes the same bounded budget a
  live one does rather than a separate unbounded one.
- Parse and build failures reach the caller verbatim rather than as a wrapped
  cause. `app.Error` normally keeps the cause for the log; here the diagnostic
  *is* the answer, so it goes in the user-facing message.
- Two things a live run does are deliberately absent, because both are the
  commit's work rather than the graph's: an inbox row minted for a key no source
  ingested (ADR function-node-per-entity-feed-items), and feed rows dropped by snapshot reconciliation. The
  outputs and `feedSnapshots` that would have driven them are reported instead,
  so what the commit *would* do is visible without a commit having a dry-run
  mode of its own.
- The trace collector is the only part of a run reachable from more than one
  goroutine — a script that outlives its interrupt keeps running and can still
  call `console.log` — so it takes a lock and caps its total lines. Everything
  else a trace records is written by the run loop alone.
