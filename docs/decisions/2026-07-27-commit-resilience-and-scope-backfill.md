# Commit resilience, pre-#63 scope backfill, and snapshot retention

- **Status:** accepted
- **Date:** 2026-07-27

## Context

#63 gave every GitHub observation a `SourceScope` (the account, e.g.
`hay-kot`). Inbox identity is the full tuple `UNIQUE (profile_id, source_kind,
source_scope, external_id)`, and every lookup keys on it. Rows written before
#63 carry `source_scope = ''`, and nothing rewrites them, so the scoped lookup
introduced by #63 cannot find them.

Two independent defects followed (issue #95):

1. **No backfill.** A pre-#63 item stays dormant until it next shows activity.
   When a flow then tries to claim feed membership for it, the scoped lookup in
   `CommitBatch` misses.
2. **One miss wedged the whole consumer.** That miss returned an error from
   inside the commit transaction, so `UpToOffset` never advanced and the engine
   retried the same page every poll, forever. Because the consumer never
   drained, `event_log` grew without bound — 1.4 GB on the reporter's install,
   dominated by full-source snapshots that are appended every poll and never
   consumed.

The failure class is more general than the scope trigger: `log.go` already
documents a second way an unresolvable item reaches the commit path (a snapshot
serialized with `omitempty` routing the boundary row as an ordinary item). A
per-trigger patch would leave the wedge reachable by the next cause.

## Decision

Three changes, ordered by how much they matter.

**1. The commit path is resilient to an unresolvable item.** A feed output
whose inbox row cannot be resolved — even after the backfill below — is skipped
and logged rather than failing the batch, so the offset advances. The next
authoritative snapshot re-attempts the claim once the row exists, so skipping is
not permanent data loss: snapshots are repeated and authoritative. This bounds
the blast radius of both the scope bug and the `log.go` one, and is what stops
`event_log` running away. The store gained a `Logger` (an `OpenOptions` field,
zero value discards) so the skip is visible.

**2. Scope is backfilled lazily by self-healing lookup, not by a migration.** A
migration cannot reliably know which account owned a pre-#63 row, and guessing
is wrong on any multi-account install. Instead, `resolveInboxItemScoped` retries
a scoped miss under the empty scope and, if that hits, rewrites the row to the
requested scope (`RescopeInboxItem`). The rewrite is safe because the caller
only reaches it when the scoped identity is free, so it never collides with the
uniqueness index. It runs both in `CommitBatch` (healing dormant items the
snapshot re-surfaces) and in `IngestObservation` (healing a changed item in
place, so a later ingest updates the row rather than forking a scoped duplicate
beside the one carrying the user's triage decisions). Convergence is one drain:
the first snapshot commit after the upgrade heals every current item at once.

**3. Superseded snapshots are pruned.** `event_log` retention already supported
age and per-topic-row bounds but both defaulted off, and neither targets the
rows that dominate size: a full-source snapshot dwarfs an item event, and every
poll appends a new one. `DeleteSnapshotsOverLimitPerTopic` keeps the newest N
snapshots per topic and deletes the rest — replay and every snapshot read take
only the newest per topic, so older ones reconcile nothing. `N ≥ 1` preserves
the single latest snapshot the age/count bounds already rely on.
`EventLogSnapshotsPerTopicLimit` defaults to 3.

## Consequences

- One unresolvable inbox item can no longer wedge a flow. A wedged-then-fixed
  consumer drains to the tail, and the snapshot bound collapses the runaway
  `event_log` back to a few megabytes on the next retention pass.
- No new migration and no schema change: the backfill is a query-level fallback
  keyed on the existing uniqueness index, and retention is a new query.
- Pre-#63 rows migrate to their account scope the first time they are resolved,
  preserving `unread`/`archived_*` — the triage decisions that
  [must survive](../architecture.md#data-that-must-survive) — because the row is
  rewritten, not replaced.
- A multi-account install where two accounts watch the same repo heals the empty
  row to whichever account resolves it first; the other account's identity is a
  genuinely new observation and is created normally by ingestion, with the
  resilience skip covering the interim.
- The skip is surfaced only in `desktop.log`, not the UI. Since the backfill
  means the pre-#63 items heal rather than skip, skips are rare; a UI/activity
  surface for them is deferred.
