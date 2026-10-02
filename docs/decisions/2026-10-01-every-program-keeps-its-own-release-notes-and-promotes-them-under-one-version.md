# Every program keeps its own release notes and promotes them under one version

- **Status:** accepted
- **Date:** 2026-10-01

## Context

The hive CLI and Hive Desktop share one repository and will ship together
under one version. Only the desktop had release notes: fragments in
`changelog/unreleased/`, promoted into `<version>.md` and embedded in the
binary (ADRs release-notes-ship-inside-the-binary,
release-notes-accumulate-as-fragments). The CLI's GitHub release body came
from GoReleaser's commit log, which in a shared repository lists every desktop
commit as well.

## Decision

Each program owns a changelog in the fragment format, beside an embed package:
`cmd/hive/releasenotes/changelog/` and
`cmd/desktop/releasenotes/changelog/`. `internal/releasenotes` holds the
parser and takes an `fs.FS`; it embeds nothing.

- `release changelog new --product <cli|desktop>` writes a fragment into one
  program's draft.
- `release changelog promote <version>` promotes every program at once. Every
  program gets a `<version>.md`, including one with no fragments, whose body
  stays empty and whose summary says the release does not change it. Every
  program's changelog then names every version it shipped under.
- `release changelog pr` refuses a promotion that lacks an entry for any
  program, or that mixes versions.

The desktop embeds its changelog and shows it in the app (ADR
release-notes-ship-inside-the-binary). The CLI's notes ship on the GitHub
release; the `hive` binary does not read them. Its embed package exists so the
release tool reads both changelogs the same way, and a CLI surface for the
notes is follow-up work.

What a release publishes is read through the embed packages. `promote` and
`pr` read the worktree instead, because the embed is fixed when the tool is
compiled, and a stale build would promote a partial draft.

## Consequences

- A pull request that changes the CLI owes a CLI fragment, the same way one
  that changes the desktop owes a desktop fragment. The `release-notes` skill
  covers both.
- The `changelog:*` mise tasks are repository-wide; the `desktop:changelog:*`
  tasks are gone.
- A CLI-only release still produces a desktop entry, and the curator writes a
  one-line summary for it.
