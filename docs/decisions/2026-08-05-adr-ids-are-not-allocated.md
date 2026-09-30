# ADR ids are not allocated

- **Status:** accepted
- **Date:** 2026-08-05

## Context

ADRs were numbered sequentially and indexed in a hand-maintained table in
`docs/README.md`. Both halves of that scheme fail under concurrent work, and one
had already failed silently.

Picking "the next number" is an allocation against state a branch cannot see.
Two branches that both pick `0072` write *different filenames*, so git merges
them cleanly and reports nothing. That is how `0046` came to name two ADRs —
`shutdown-is-signalled-and-bounded` and
`terminal-first-paint-carries-scrollback` — with `docs/architecture.md` citing
`ADR terminal-first-paint-carries-scrollback` on one line to mean the first and on another to mean the second.
Nothing in CI could see it, because nothing was in conflict.

The index conflicted for the opposite reason: every ADR appended a row to the
bottom of the same table, so any two concurrent ADRs collided on the same lines.
A guaranteed conflict on every ADR, and a silent duplicate whenever the number
race was lost.

Nothing validated either. A citation in `ephemeral-popup-terminals` read
`[ADR ptyterm-terminals-are-caller-addressed](0060-ptyterm-terminals-are-caller-addressed.md)` — a number and a
path that disagreed, pointing at a file that did not exist. Three ADRs carried
their metadata as plain `Status:`/`Date:` lines rather than the bullet form the
other 71 used.

## Decision

**An ADR's id is its filename, `YYYY-MM-DD-slug.md`. Nothing allocates it.** The
date orders the directory and the slug names the decision; uniqueness comes from
the slug, since a same-day collision requires two branches to write the same
decision twice — which is a real conflict, not a bookkeeping one.

**Prose cites the slug alone**: `(ADR terminal-transport)`. The date lives in
the filename and the link, not in the sentence — the ~525 inline citations are
terse parentheticals, and a full `2026-07-28-terminal-transport` roughly triples
them and forces rewraps in Go comments and table cells. This makes slugs the
citation handle, so `check:adr` requires them to be globally unique, not merely
unique within a date.

~~**The index is generated.** `docs/decisions/README.md` is built from the ADRs
themselves by `cmd/adr` and is never hand-edited. It carries `merge=union` in
`.gitattributes`: concurrent additions take both sides' rows instead of
conflicting, and regeneration reorders and dedupes afterward.~~ The index did
not survive: a union merge lands both sides' rows unordered, nothing after the
merge reruns the regeneration, and the "stale index" failure surfaces on
whichever branch touches `docs/decisions/` next. The table duplicated the
directory listing, so it was removed rather than repaired — the listing is the
index.

**`cmd/adr` is the gate.** `adr new` writes the file so the name is never typed
by hand; `adr check` runs in `mise run check` and CI, and fails on a malformed
id, a filename whose date disagrees with its `Date` field, a duplicate slug, a
citation or link that does not resolve, or a legacy numbered reference.

All 74 existing ADRs were renamed and their ~640 references rewritten in one
change, rather than freezing the old numbers and running two schemes side by
side. The `0046` citations were the only ones that could not be rewritten
mechanically; each was resolved by reading its context.

## Consequences

- Adding an ADR no longer conflicts with another branch adding one. ~~The index
  is the only shared file, and it is generated and union-merged.~~ With the
  index removed, there is no shared file at all.
- Numbers are gone as a sort key. Chronology comes from the filename prefix,
  which is why the `Date` field and the filename are checked against each other
  — a wrong date silently misfiles the ADR.
- Renaming an ADR breaks every citation to it. That is deliberate and now loud:
  `check:adr` names each dangling reference, so the rename and its citations
  land together.
- References in this repo's history and in merged PR descriptions still say
  `ADR terminal-transport`. They are not rewritten, and the numbers no longer resolve; the
  directory listing maps a date to a slug for anyone following an old thread.
- `check:adr` shells out to `git ls-files` to find citation sites, so it must run
  inside the checkout. It is not part of the pre-commit hook — only `check` and
  CI — because it scans every tracked text file.
- Released SQLite migrations are pinned byte-for-byte by `check:migrations`, so
  the numbered citations in their comments could not be rewritten and are exempt
  from `check:adr`. One of them, in `0007_item_session.up.sql`, cites `ADR macos-dmg-installer`
  where it means the item↔session decision — a typo that predates this change
  and cannot be corrected without breaking the immutability gate.
- A backticked id is read as a quotation, not a citation, which is what lets this
  ADR and the docs discuss the retired numbers without failing the gate.
