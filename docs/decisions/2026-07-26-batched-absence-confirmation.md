# Batched, keyed GitHub absence confirmation

- **Status:** Accepted
- **Date:** 2026-07-26

## Context

Absence confirmation cost one uncached REST GET per absent item, serially,
every tick, over a set that never shrank: `Producer.confirmAbsent` listed
every `source_head` key still untouched by the tick's observations and called
`ConfirmAbsence` on each, one REST call apiece, no cache, no conditional
request, no singleflight. At the default 60s tick and a 5,000 req/hour
authenticated budget, ~83 absent keys exhausts the REST quota; a few days of
normal use crosses that. `AbsenceVerdict.Terminal` was already computed and
discarded — nothing evicted a settled item, so the set only grew.

## Decision

Batch confirmation into one aliased GraphQL document per 100 items
(`repository(owner,name){issueOrPullRequest(number)}`, chunked at
`ghclient.ItemStates`), and return verdicts keyed by external id across the
connector boundary rather than positionally — a chunk failure must not
silently misalign the rest. A terminal verdict evicts the item's
`source_head` row permanently, so a settled item is confirmed once, not every
tick forever. The tracked set is bounded to active, non-pruned inbox rows;
`ListSourceHeadKeys` no longer returns rows an archival prune already deleted
from `inbox_item`, and any row left orphaned by that gap is reclaimed at
retention rather than tracked indefinitely.

Rejected alternatives:

- `nodes(ids:)` — needs a persisted GraphQL node id, which notification-sourced
  items never have.
- Dropping `is:open` from starter search queries — would spend the feed's
  fixed per-tick window slots on items that are already closed.

## Consequences

- N absent items cost `ceil(N/100)` GraphQL requests per tick, at roughly 1-2
  points each against the GraphQL rate limit, instead of N uncached REST
  calls.
- An item whose repository is deleted or made private resolves null forever —
  never terminal — so it stays in the feed until manually archived. That is
  deliberate: a briefly-private repo must not get treated as merged/closed and
  archived out from under the user.
- A `source_head` row whose key still exists under a different profile survives
  retention as a harmless orphan rather than being torn down mid-use.
