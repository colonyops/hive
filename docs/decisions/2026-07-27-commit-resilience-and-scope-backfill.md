# Commit resilience, pre-#63 scope backfill, and snapshot retention

- **Status:** accepted
- **Date:** 2026-07-27

## Context

Issue #63 gave every GitHub observation a `SourceScope` (the account). Inbox
identity is `UNIQUE (profile_id, source_kind, source_scope, external_id)` and every
lookup keys on it. Rows written before #63 carry `source_scope = ''`, so a
scoped lookup misses them.

Issue #95 showed what followed: one unresolvable item returned an error inside
the commit transaction, the consumer offset never advanced, the engine retried
the same page forever, and `event_log` grew without bound with full-source
snapshots appended every poll and never consumed.

## Decision

1. **The commit path never fails a batch on an unresolvable feed output.** A
   keyless output has no identity, so `CommitBatch` logs and skips it. A keyed
   output with no inbox row is minted from its payload (ADR
   function-node-per-entity-feed-items). Either way the offset advances. The
   store reports skips through `OpenOptions.Logger`.

2. **Scope is backfilled lazily by a self-healing lookup, not a migration.** A
   migration cannot know which account owned a pre-#63 row on a multi-account
   install. `InboxItemStore.ResolveScoped` retries a scoped miss under the
   empty scope and, on a hit, rewrites the row (`RescopeInboxItem`) and its
   `item_session` links to the requested scope in one transaction. The rewrite
   cannot collide with the uniqueness index because it runs only after the
   scoped identity missed. `CommitBatch` (through `ResolveScopedID`) and
   `IngestObservation` both use it, so a later ingest updates the healed row
   instead of forking a scoped duplicate.

3. **Superseded snapshots are pruned.** `DeleteSnapshotsOverLimitPerTopic`
   keeps the newest N snapshots per topic. Replay and every snapshot read use
   only the newest, so older ones reconcile nothing.
   `EventLogSnapshotsPerTopicLimit` defaults to 3; any positive value keeps the
   single latest snapshot the age and count bounds rely on.

## Consequences

- A pre-#63 row keeps its `unread` and `archived_*` state when healed,
  because it is rewritten, not replaced
  ([data that must survive](../architecture.md#data-that-must-survive)).
- When two accounts watch the same repo, the empty-scope row heals to the
  account that resolves it first. The other account's identity is a new
  observation and ingest creates it normally.
- Skips show only in `desktop.log`.
