# RSS feed

An **RSS feed** node fetches one feed document on the poll tick and ingests its entries as items. RSS, Atom and JSON Feed all go in the same `url` field; the parser reads the document, not the file extension. It has no inputs; this is where a flow starts.

Its output is a **window**, not a snapshot. A publisher drops old entries as new ones arrive, and `limit` cuts the list further, so an entry that stops appearing has scrolled off the end rather than been resolved. Nothing is archived when that happens -- the entry stays until retention ages it out.

The first fetch of a new node ingests everything still in the window, up to `limit`. Point a notify node at a busy feed and the first tick is the loud one.

## Fields

- `url` -- required. The feed document, `http` or `https`. It must be readable without credentials: this node sends no token and no basic auth. A URL that carries its own `?token=` works, but it puts a secret in a file you probably commit.
- `limit` -- optional. How many of the feed's most recent entries to ingest per fetch. Empty is 50, the maximum is 500. Entries are ordered newest first by their published date, falling back to their updated date; entries the feed dated neither way sort last in the order it listed them.
- `interval` -- optional, e.g. `30m`. The shortest time between fetches. The feed still only loads on a poll tick, so the real cadence rounds up to the next one; empty fetches on every tick. It is not persisted -- a restart fetches every source once.
- `icon` -- optional glyph, from the curated feed icon set, shown on this source's items. Empty uses the default feed glyph.
- `image` -- optional uploaded image (the site's logo) shown as this source's mark instead of the glyph. It is a content hash of a normalized PNG kept in the app data dir, set through the editor's image picker; it is presentation only and never affects ingest. Empty falls back to `icon`.

## What an entry becomes

Each entry is emitted as one item against the canonical item contract:

- **Identity** is the entry's `<guid>` (RSS) or `<id>` (Atom). Without one it is the entry's link, and without that a digest of its title and date -- which makes an edited title a new item, so a feed that publishes no ids is the one case where duplicates are possible.
- `kind` is `Post`, so `applies_to: [Post]` targets a feed entry whichever feed it came from.
- `title`, `url` and `author` come from the entry.
- `repo` is the **feed's** title, which is what the row renders above the entry.
- `body` is the entry's summary, or its content when it publishes no summary. HTML becomes markdown text, cut at 4000 characters: tags go, links stay as markdown links, and a block boundary becomes a space. This is a description, not a reader. Plenty of feeds publish no summary at all, and one that puts only a link in its summary (Hacker News points at its comments page) gives you that link and nothing else.
- `labels` are the entry's categories, at most 20.
- `published` and `updated` are RFC 3339 timestamps, absent when the feed omits them.

There is no `state`. A feed entry has no lifecycle to report, so nothing here ever archives itself -- route on `published`, or archive by hand.

## Fetching

Each fetch sends the `ETag` and `Last-Modified` of the previous one. A feed that answers `304 Not Modified` costs one round trip and no parse, and the previous window is re-emitted unchanged. **Set an `interval`** anyway: a conditional request is cheap, not free, and the default poll tick is far more often than any feed publishes.

Two nodes pointing at the same URL share one fetch and one cache. A manual refresh drops both, so a feed whose `ETag` went stale can still be forced.

## Failure

A fetch either produces the whole window or fails. These are all failures, not empty windows:

- the host is unreachable, or the request times out,
- a non-2xx response, including a 404 for a feed that moved,
- a document larger than 8 MiB,
- a document the parser cannot read as a feed -- an HTML error page served with a 200, most often.

A failed fetch changes nothing: the previous window stays in place, nothing is archived, and nothing is emitted. It is recorded in Activity. A source that keeps failing is re-announced at most once an hour until it succeeds.
