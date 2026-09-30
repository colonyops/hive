# An RSS window is not an authoritative set

- **Status:** proposed
- **Date:** 2026-09-12

## Context

Every pull connector so far returns the complete current set from `Produce`.
The producer relies on it: an item that was in the source head and is not in
this tick's result is absent, and a connector that declares
`CapConfirmAbsence` is asked what happened to it. `sources.exec` leans on this
hardest -- stdout is the whole snapshot by contract, so an item that stops
appearing is resolved.

A feed is not that. A publisher keeps the last 10, 20 or 50 entries and drops
the rest as it writes new ones, and `limit` cuts the list further. An entry
that left the window has scrolled off the end; nothing about it changed. Wiring
a feed up the way `sources.exec` is wired would archive last month's posts as
"resolved" on the tick they fall out, and re-archive them every time the
publisher's pagination wobbles.

Conditional requests put the same trap on the fetch path. `304 Not Modified` is
the answer a healthy feed gives most of the time, and a connector that returns
no entries for it hands the producer an empty set, which is read as "this feed
published nothing" and archives the whole window.

## Decision

**`sources.rss` produces a window, and the app treats absence from it as
meaningless.**

1. The descriptor declares **no capabilities**. No `CapConfirmAbsence`, so
   nothing asks what happened to a departed entry; no `CapClassify`, so the
   generic observed/updated classifier applies. An entry that leaves the window
   stays as it was until retention ages it out.
2. An entry payload carries **no `state`**. A feed entry has no lifecycle to
   report, and the canonical `state` vocabulary archives on `resolved`,
   `closed` and `done` -- so a connector that guessed one would be inventing a
   lifecycle the publisher never described.
3. A **304 re-emits the cached window**, never an empty one. The validators and
   the parsed entries are one cache entry, written and dropped together, so a
   304 can always be answered. A 304 arriving with nothing cached retries
   unconditionally rather than emitting nothing.
4. Identity is the entry's `<guid>` or Atom `<id>`, then its link, then a digest
   of its title and date. The digest makes an edited title a new item. That is
   wrong and bounded; keying on position or on nothing would make every fetch a
   new set of items.

**The first cut reads public feeds only.** No `credential` field and no
provider, because the alternative that costs nothing -- a `headers:` map in the
node -- writes a secret into a file meant for a dotfiles repo, which
[`architecture.md`](../architecture.md#source-connectors) forbids, and the
alternative that does it properly is a new credentials provider and a new
Integrations drawer. Auth is additive: a `credential` field and a scheme can be
added later without moving anything decided here.

## Consequences

- A feed source never archives anything by itself. Automation that wants an
  entry gone acts on it, or lets retention do it.
- The first fetch of a new node ingests the whole window at once. `limit`
  defaults to 50 to bound that burst; the node documentation says so, because a
  notify node on a busy feed makes the first tick loud.
- The cache is in memory, so a restart refetches every feed once. That is one
  request per feed, and the alternative is persisting validators against
  entries that were never persisted with them.
- A feed that publishes no ids and no links gets duplicate items when a title
  is edited. There is no fix available from the feed itself; it is recorded here
  so it reads as a known cost rather than a bug.
