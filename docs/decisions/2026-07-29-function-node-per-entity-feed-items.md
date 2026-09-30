# Per-entity feed items by function-node fan-out

- **Status:** accepted
- **Date:** 2026-07-29

## Context

A source emits one message per node. `sources.grafana_metrics` emits one
message keyed by the node id carrying the whole query result, so a query whose
result is N series maps to one durable feed item, not N. Issue #117 wants the
opposite for a class of workflows where each series is an actionable entity: one
feed item per series, each with its own key, payload, triage state, and
lifecycle — the shape `sources.grafana_alerts` already has per fingerprint.

The intended fix is a *general* one, authored in JavaScript rather than built
into a connector: a `function` node splits one message into many. The engine
already supports the multiplicity — a single-output function returning an array
fans every element onto port 0. What blocked it was identity. Feed membership
resolves against an `inbox_item` row keyed by `(profile, source kind, source
scope, external id)`, and those rows are minted only at the ingest boundary
(`IngestObservation`) from the keys the *source* emits. A key a function node
minted has no row, so `CommitBatch` resolved it to nothing and — under ADR
0030's resilience rule — skipped the claim. The per-entity items silently never
appeared.

## Decision

**A feed output whose key has no inbox row is minted at commit, from the payload
it carried.** `feedSinks` now carries `msg.Payload` on the output; when
`CommitBatch`'s feed branch resolves a non-empty key to no row, it inserts one
(title, url and `updatedAt` read from the payload the same way the ingest
boundary reads them — one decode, `models.FeedFields` — lifecycle `active`) and
claims membership against it. A payload that states no `updatedAt` is stamped at
mint time, never at the epoch. A key the producer already
ingested still resolves to its classifier-owned row and is left untouched, so
this changes nothing for ordinary source items.

This **supersedes ADR commit-resilience-and-scope-backfill's point 1 for non-empty keys**: an unresolvable feed
output is now minted rather than skipped. The anti-wedge property that decision
protects is preserved because minting succeeds — the offset still advances. A
key-*less* output (the `omitempty` snapshot-boundary row of issue #95) has no
identity to mint under and is still skipped and logged.

**The key is the author's to mint; the topic is not.** The split messages
inherit the source snapshot's reconciliation scope — its topic and snapshot id —
through the function node, so lifecycle is presence-based for free: every poll
restates the current set, and a series that leaves the query has its membership
claim reconciled away on the next snapshot. Rewriting `msg.Topic` would move the
output out of that scope and break reconciliation, so the function-node contract
is: mint `Key`, preserve `Topic`.

**Lifecycle is membership drop, not absence-confirmed resolution.** A departed
series' item loses its feed claim and becomes unfiled (Trash); it is not
archived with a "resolved" reason the way an alert is. A synthesized key never
reaches the source or ingest layer, so no `AbsenceConfirmer` applies to it. The
item's row and triage state persist, so the series reappearing re-claims the
same item cleanly.

## Consequences

- `sources.grafana_metrics` is unchanged. Per-series identity and lifecycle are
  authored in a function node, so the same mechanism serves any source whose one
  message carries many entities — no per-connector code, no split node type.
- Minting happens only on the live commit path. Deploy-time replay resolves feed
  outputs against the *existing* unarchived inbox and skips what it cannot find
  (`Engine.replay`), so a synthesized item first appears on the next live poll
  after its snapshot, and — once minted — survives redeploys because replay
  reinstalls its claim. This is consistent and self-healing, not a special case.
- A synthesized item's payload is frozen at first appearance: a later poll
  re-claims the existing row without refreshing its title, payload or
  `updatedAt`, and no `inbox_event` history is recorded. Feeds are membership surfaces that never
  notify, so neither matters for the feed. The per-entity payload the use case
  needs (a series' identity labels, for an `applies_to` action) is static, so
  freezing it is correct rather than merely tolerable. Refresh-on-change can be
  added if a use case needs it.
- Cardinality is the author's responsibility: an unbounded-cardinality query is
  an unbounded feed. A per-node series cap is a plausible future guard; it is
  not enforced today.
- The original one-message-per-node item (keyed by the source's own key) still
  exists as an ingested row; when a function node replaces it with per-entity
  items, that row is simply never claimed by the feed and stays unfiled.
