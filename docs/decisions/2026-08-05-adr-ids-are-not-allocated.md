# ADR ids are not allocated

- **Status:** accepted
- **Date:** 2026-08-05

## Context

Sequential ADR numbers are an allocation against state a branch cannot see. Two
branches that pick the same number write different filenames, git merges them
cleanly, and the number ends up naming two decisions. A hand-maintained index
table conflicts on every concurrent ADR.

## Decision

**An ADR's id is its filename, `YYYY-MM-DD-slug.md`. Nothing allocates it.**
The date orders the directory and the slug names the decision. The directory
listing is the index; there is no generated table.

**Prose cites the slug alone**: `(ADR terminal-transport)`. Slugs are the
citation handle, so they must be globally unique, not merely unique within a
date.

**`cmd/tools/adr` is the gate.** `mise run adr:new` writes the file so the name
is never typed by hand. `mise run check:adr` runs in `mise run check` and CI and
fails on a malformed filename, a `Date` field that disagrees with the filename,
a duplicate slug, a citation or link that does not resolve, or a legacy
numbered reference.

## Consequences

- Adding an ADR never conflicts with another branch adding one.
- The `Date` field and the filename are checked against each other because a
  wrong date silently misfiles the ADR.
- Renaming an ADR breaks every citation to it, loudly: `check:adr` names each
  dangling reference, so the rename and its citations land together.
- Old numbered references in git history and merged PR descriptions do not
  resolve.
- `check:adr` uses `git ls-files` to find citation sites, so it runs inside the
  checkout, and it is not in the pre-commit hook because it scans every tracked
  text file.
- Released desktop migrations
  (`cmd/desktop/internal/app/data/queries/migrations/`) are pinned
  byte-for-byte, so their numbered citations are exempt from `check:adr` and
  stay as written.
- A backticked id is a quotation, not a citation.
