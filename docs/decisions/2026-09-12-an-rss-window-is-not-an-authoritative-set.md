# An RSS window is not an authoritative set

- **Status:** accepted
- **Date:** 2026-09-12

## Context

The producer treats a pull connector's `Produce` result as the complete
current set: an item missing from this tick is absent, and a connector that
declares `CapConfirmAbsence` is asked what happened to it. A feed is not that
set. A publisher keeps its last few entries and drops older ones, and `limit`
cuts further, so an entry that left the window only scrolled off the end. A
`304 Not Modified` that produced no entries would read as "this feed
published nothing".

## Decision

**`sources.rss` produces a window, and absence from it means nothing.**

1. The descriptor declares **no capabilities**: no `CapConfirmAbsence`, so
   nothing asks about a departed entry, and no `CapClassify`, so the generic
   classifier applies. An entry that leaves the window stays until retention
   ages it out.
2. An entry payload carries **no `state`**. A feed entry has no lifecycle,
   and the canonical vocabulary archives on `resolved`, `closed` and `done`.
3. A **304 re-emits the cached window**, never an empty one. The validators
   and the parsed entries are one cache entry, written and dropped together.
   A 304 with nothing cached retries unconditionally.
4. Identity is the entry's `<guid>` or Atom `<id>`, then its link, then a
   digest of its title and date.

**Feeds are public only.** A `headers:` map would write a secret into a
file meant for a dotfiles repo, and a URL with userinfo is refused for the
same reason. Auth can be added later as a `credential` field without
changing anything here.

## Consequences

- A feed source never archives anything by itself. Automation acts on an
  entry, or retention ages it out.
- The first fetch ingests the whole window. `limit` defaults to 50 (at most
  500) to bound that burst.
- The cache is in memory, so a restart refetches every feed once.
- A feed with no ids and no links gets a new item when a title is edited.
  That is a known cost of the digest fallback.
