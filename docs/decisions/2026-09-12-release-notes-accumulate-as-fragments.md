# Release notes accumulate as fragments

- **Status:** accepted
- **Date:** 2026-09-12

## Context

Every user-visible change appended a bullet to
`internal/app/releasenotes/changelog/next.md`
(ADR [release-notes-ship-inside-the-binary](2026-08-06-release-notes-ship-inside-the-binary.md)).
Appending to the tail of `## Added` is the same edit every branch makes, so any
two branches carrying a note conflict by construction. That is now most of
them: of the 34 commits that have ever touched the file, seven landed on
2026-09-09 — every commit that day — and four of six on 2026-08-29. Agents
working concurrently hit it on nearly every pull request.

Fragment-per-change is the settled answer elsewhere: towncrier keys fragments
on an issue number, scriv on `<timestamp>_<author>_<branch>`, changie on a UTC
timestamp, changesets on a random name. All of them then roll the fragments up
at release.

The roll-up is where the shape is decided, and this repository has already run
one of the two experiments. `docs/decisions/README.md` was a generated index
with `merge=union`, and it failed: a union merge lands both sides unordered,
nothing reruns the generator afterwards, and the breakage surfaces on whoever
touches the directory next (ADR [adr-ids-are-not-allocated](2026-08-05-adr-ids-are-not-allocated.md)).
A generated `next.md` committed to the tree is that same design and conflicts
as much as the hand-written one it replaces.

## Decision

**The draft is a directory.** `changelog/unreleased/` holds one file per
change, named `<YYYYMMDDThhmmss>-<slug>.md`, and there is no `next.md`.
Nothing allocates the name: the timestamp is UTC so fragments written in
different timezones still sort into the order they were written, and the slug
comes from the note. A collision needs two branches to write the same note in
the same second.

**The filename carries nothing the renderer reads.** It is an ordering key and
a unique name; the section a note belongs to is `kind` in its frontmatter, and
that is the only copy. Putting the kind in the name as well would make a diff
say more, at the price of a rule that keeps two spellings of one fact from
drifting apart, which is not a trade worth making for a file listing.

**The roll-up is not a file.** `go:embed` takes the fragment directory and
`Load` renders the draft entry in memory, so no generated artifact is
committed and there is nothing left in the tree for two branches to conflict
over. The rendered body is the same markdown the hand-written draft carried —
`## Added` / `## Changed` / `## Fixed` in that order, fragments ordered by
filename within a section — which is what lets promotion keep moving bytes
rather than re-deriving them. `Entry`, `Entries.Draft` and everything
downstream of them are unchanged.

**`release changelog new` writes the file** so the name is never typed, the
way `adr new` does. `mise run changelog:new -- --kind added "..."`.

**The release-notes pull request is a command, not a procedure.** `changelog
promote` requires a clean `main` so that everything the worktree holds
afterwards is the promotion, and `changelog pr` then reads the worktree,
refuses anything that is not the promotion, and creates the branch, the commit
and the pull request itself. Nothing about the shape of that commit is left to
whoever runs the release. `changelog pr` is also where an entry whose `summary`
is still empty is caught; `notesFor` repeats the check, for an entry that
reached `main` some other way.

**Promotion is the curation point.** A release assembled verbatim from
fragments reads as the sum of pull requests, because a branch cannot consolidate
against a bullet it never sees — the cost every fragment system pays, and the
reason Astronomer demoted towncrier's output to an internal digest a writer
rewrites. These notes are product copy shown in-app, so the consolidation the
`release-notes` skill used to ask of each pull request moves to
`changelog promote`: it writes `<version>.md` with an empty `summary` and the
entry is edited before it is committed.

The draft loses its `summary` for the same reason. It described the whole
release, which no single change can write; it has been `""` in practice, and
it is now written at promotion with the rest of the curation.

## Consequences

- Two branches can each land a release note with no conflict. That was the
  point.
- What dev and beta users read is no longer byte-identical to the stable entry
  that follows it. Promotion already had to prune reverted work, so this widens
  an editing step that existed rather than adding one, but a stable release's
  notes are now authored rather than accumulated.
- Fragments order by time, not by importance. A promoted entry that wants a
  different order gets it by hand, at promotion.
- `changelog/unreleased/.gitkeep` has to exist. Git cannot track an empty
  directory, and `go:embed changelog` drops a directory whose only entry starts
  with `.` — so between a promotion and the next change the embedded FS has no
  `unreleased/` at all, which `Fragments` reads as "no draft".
- A fragment is not reviewable as a rendered changelog. What a pull request
  shows is one file; what the release says is only assembled at build time.
  `TestChangelogParses` covers the parse, not the reading. A file listing does
  not say which section a note lands in either — that is in the file.
- Notes for work that never shipped are deleted by deleting a file, rather than
  by editing a bullet out of a shared one.
