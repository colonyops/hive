# Command source

A **command source** node runs a shell command on every poll tick and ingests what it prints as this node's current items. It turns any CLI that can produce JSON into a source — no external scheduler, no webhook endpoint, no state file. It has no inputs; this is where a flow starts.

Its output is a **snapshot**: what the command prints is the complete current set. An item that stops appearing is treated as gone and archived, so the feed follows reality without the command tracking what changed since last time.

## Fields

- `command` — required. The command line, run through `sh -c`, so pipes, redirection, and `&&` work. It runs with the PATH your login shell reports (not the desktop's), your own environment, and no shell aliases — an alias is interactive-shell sugar and does not resolve here.
- `timeout` — required, e.g. `30s`. How long one run may take before it is killed and the tick fails. At most `2m`: a tick drains sources in sequence, so this budget is taken out of every other source's freshness.
- `cwd` — optional. Absolute path (or one starting with `~/`) to run in. Empty runs in the app's own directory, which for a launched app is not a useful place — set this if the command cares.
- `env` — optional map of extra environment variables, added to the inherited environment. Values are literal: nothing is expanded or interpolated.
- `interval` — optional, e.g. `1h`. The shortest time between runs, for a command that is expensive or only worth running occasionally. The command still only runs on a poll tick, so the real cadence rounds up to the next one; empty runs it every tick. It is not persisted — a restart runs every source once.
- `icon` — optional glyph, from the curated feed icon set, shown on this source's items. Empty uses the default command glyph.
- `image` — optional uploaded image (the tool's logo) shown as this source's mark instead of the glyph. It is a content hash of a normalized PNG kept in the app data dir, set through the editor's image picker; it is presentation only and never affects ingest. Empty falls back to `icon`.

## Output contract

- Print a **JSON array of objects** on stdout and exit `0`. The array is the whole snapshot: `[]` is a legitimate empty one, and it archives every item this source owns.
- Item identity: each object needs a top-level `"id"` (string or number). It is the stable key — the same id on the next run updates the same item; a new id is a new item. This is the one field a command must supply, because without it a changed item is indistinguishable from a new one.
- A top-level `"title"` and `"url"` are promoted so the item renders in feeds; everything else stays in the opaque `msg.Payload` for downstream nodes, decoded against the canonical item contract (docs/decisions/0008) wherever it renders.
- A top-level `"kind"` is the item's type label and what actions target with `applies_to`. Omit it and the item is kind `Item` — still automatable (`applies_to: [Item]`), just not distinguishable from other untyped items.
- A top-level `"state"` drives lifecycle: `resolved`, `closed`, and `done` (case-insensitive) system-archive the item with the state as the archive reason; any other or absent state keeps it active. An item whose state leaves one of those values resurfaces.
- This is the same item contract a webhook delivery carries, so a payload written for one works in the other.

If the command emits one JSON object per line, pipe it through `jq -s .` to make an array.

## Failure

A run either produces the whole snapshot or fails; there is no partial ingest. These are all failures, not empty snapshots:

- a non-zero exit,
- no output at all (an empty snapshot must be printed as `[]`),
- stdout that is not a JSON array — including `null`, a bare object, or NDJSON,
- an item with no `id`, two items sharing one, or an array entry that is not an object,
- more than 1 MiB on stdout, or exceeding `timeout`.

A failed run changes nothing: the previous snapshot stays in place, no item is archived, and nothing is emitted. It is recorded in Activity with the exit status and an excerpt of stderr. A source that keeps failing is re-announced at most once an hour until it succeeds, so a broken command does not bury the log.

The failure this contract exists to prevent is the quiet one: without it, a command that broke would print nothing, ingest as an empty snapshot, and silently archive every item the source owns.

## Trust

This node runs a command on your machine, on a timer, with your environment. Flow files are meant to live in a dotfiles repo, so treat one that arrives from elsewhere the way you would treat a shell script from the same place. The command is a fixed string and is never templated from ingested data, so nothing this or any other node fetches can change what runs.
