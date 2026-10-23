# Every program keeps its own release notes and promotes them under one version

- **Status:** accepted
- **Date:** 2026-10-01

## Context

Every program in the repository ships under one version
(ADR every-program-ships-under-one-shared-version). A GitHub release body
built from the commit log of a shared repository lists every program's
commits, so each program needs notes of its own in the fragment format
(ADR release-notes-accumulate-as-fragments).

## Decision

Each program owns a changelog beside an embed package:
`cmd/hive/releasenotes/changelog/`, `cmd/desktop/releasenotes/changelog/`,
and `cmd/drive-server/releasenotes/changelog/`. `internal/releasenotes` holds
the parser and takes an `fs.FS`; it embeds nothing. The release tool lists
the programs in `cmd/tools/release/product.go`.

- `release changelog new --product <name>` writes a fragment into one
  program's draft.
- `release changelog promote [version]` promotes every program at once.
  Every program gets a `<version>.md`, even one with no fragments, so every
  changelog names every version it shipped under.
- `release changelog pr` refuses a promotion that lacks an entry for any
  program, or that mixes versions.
- `release changelog notes <version>` prints every program's entry as the
  GitHub release body.

What a release publishes is read through the embed packages. `promote` and
`pr` read the worktree instead, because the embed is fixed when the tool is
compiled and a stale build would promote a partial draft.

Only the desktop shows its notes in the binary
(ADR release-notes-ship-inside-the-binary). The other programs' notes ship
on the GitHub release.

## Consequences

- A pull request owes a fragment to each program it changes. The
  `release-notes` skill covers this.
- A release that changes one program still produces an entry for every
  other, and the curator writes a one-line summary for each.
