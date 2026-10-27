# Per-entity feed items by function-node fan-out

- **Status:** accepted
- **Date:** 2026-07-29

## Context

A source emits one message per node. `sources.grafana_metrics` emits one
message carrying the whole query result, so N series map to one feed item.
Issue #117 wants one feed item per series, each with its own key, payload,
triage state and lifecycle. The general fix is a `function` node that splits
one message into many (a single-output function returning an array fans every
element onto port 0). What blocked it was identity: inbox rows are minted only
at ingest (`IngestObservation`) from keys the source emits, so a key a function
node minted resolved to nothing at commit.

## Decision

**A feed output whose key has no inbox row is minted at commit, from the
payload it carried.** `feedSinks` carries `msg.Payload` on the output. When
`CommitBatch`'s feed branch resolves a non-empty key to no row, it creates one
(`CreateSynthesized`: title, url and `updatedAt` read with
`models.FeedFields`, stamped at mint time when the payload has no
`updatedAt`) and claims membership against it. A key the producer already
ingested resolves to its existing row, which is left untouched. A keyless
output is still skipped (ADR commit-resilience-and-scope-backfill).

**The function node mints `Key` and preserves `Topic`.** Split messages
inherit the source snapshot's topic and snapshot id, so lifecycle is
presence-based: every poll restates the current set, and a series that leaves
the query loses its claim on the next snapshot. Rewriting `msg.Topic` moves the
output out of that scope and breaks reconciliation.

**Lifecycle is membership drop, not absence-confirmed resolution.** A departed
series' item becomes unfiled; it is not archived as resolved. A synthesized key
never reaches the source layer, so no `AbsenceConfirmer` applies. The row and
its triage state persist, so a returning series re-claims the same item.

## Consequences

- Minting happens only on the live commit path. Deploy-time replay
  (`Engine.replay`) works from existing unarchived rows, so a synthesized item
  first appears on the next live poll and, once minted, survives redeploys.
- A synthesized item's payload is frozen at first appearance. A later poll
  re-claims the row without refreshing title, payload or `updatedAt`, and
  records no `inbox_event`.
- Cardinality is the author's responsibility: an unbounded query is an
  unbounded feed. No per-node cap is enforced.
- The source's own one-message item still exists as an ingested row; when a
  function node replaces it, the feed never claims it and it stays unfiled.
