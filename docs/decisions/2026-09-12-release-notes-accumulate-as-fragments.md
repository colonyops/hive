# Release notes accumulate as fragments

- **Status:** accepted
- **Date:** 2026-09-12

## Context

Release notes ship inside each binary
(ADR release-notes-ship-inside-the-binary). When every change appends a
bullet to one shared draft file, any two branches that carry a note conflict
by construction, and concurrent agents hit it on nearly every pull request.
A generated draft committed to the tree conflicts just as much: a union merge
lands both sides unordered and nothing reruns the generator
(ADR adr-ids-are-not-allocated).

## Decision

**The draft is a directory.** Each program's `releasenotes/changelog/unreleased/`
(`cmd/hive`, `cmd/desktop`, `cmd/drive-server`) holds one file per change,
named `<YYYYMMDDThhmmss>-<slug>.md`. The timestamp is UTC so fragments sort
in the order they were written, and the slug comes from the note.

**The filename carries nothing the renderer reads.** The section is `kind`
(`added`, `changed`, `fixed`) in the frontmatter, and that is the only copy.

**The roll-up is not a file.** `go:embed` takes the changelog directory and
`internal/releasenotes` renders the draft entry in memory, `## Added` /
`## Changed` / `## Fixed` in that order, fragments ordered by filename within
a section. No generated artifact is committed.

**`mise run changelog:new` writes the file**, so the name is never typed.

**Promotion is a command and the curation point.** `changelog:promote`
requires a clean `main` equal to `origin/main`, and writes `<version>.md`
with an empty `summary`. A release assembled verbatim from fragments reads as
the sum of pull requests, so the entry is edited (consolidated, pruned, given
a summary) before it is committed. `changelog:pr` refuses anything in the
worktree that is not the promotion, refuses an entry with no summary, and
creates the branch, commit and pull request.

## Consequences

- Two branches can each land a release note with no conflict.
- What dev and beta users read is not byte-identical to the stable entry
  that follows it; a stable release's notes are authored at promotion.
- Fragments order by time, not importance. A different order is set by hand
  at promotion.
- `unreleased/.gitkeep` must exist, because git does not track an empty
  directory; without it `Fragments` reads a missing `unreleased/` as no draft.
- A pull request shows one fragment, not the rendered changelog. Tests cover
  the parse, not the reading.
