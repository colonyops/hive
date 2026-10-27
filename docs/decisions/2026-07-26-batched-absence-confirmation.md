# Batched, keyed GitHub absence confirmation

- **Status:** accepted
- **Date:** 2026-07-26

## Context

Absence confirmation asks GitHub what happened to an item that left a source's snapshot. One uncached
REST GET per absent item per tick, over a set that never shrinks, exhausts the REST quota within days of
normal use.

## Decision

- **Batch** confirmation into one aliased GraphQL document per 100 items
  (`repository(owner,name){issueOrPullRequest(number)}`), chunked in `ghclient.ItemStates`.
- **Key verdicts by external id** across the connector boundary, not by position, so a failed chunk
  cannot misalign the rest.
- **A terminal verdict evicts the item's `source_head` row**, so a settled item is confirmed once.
- **Bound the tracked set** to active inbox rows: `ListActiveSourceHeadKeys` joins `inbox_item` and skips
  archived rows. Retention deletes `source_head` rows whose key no longer exists in `inbox_item`
  (`DeleteOrphanedSourceHeads`).

`nodes(ids:)` does not work here: it needs a persisted GraphQL node id, which notification-sourced items
never have.

## Consequences

- N absent items cost `ceil(N/100)` GraphQL requests per tick.
- An item whose repository is deleted or made private resolves null forever, never terminal, so it stays
  in the feed until archived by hand. A briefly private repo must not be treated as closed and archived.
- A `source_head` row whose key still exists under another profile survives retention as a harmless
  orphan.
