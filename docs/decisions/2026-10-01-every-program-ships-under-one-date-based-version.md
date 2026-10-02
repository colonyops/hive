# Every program ships under one date-based version

- **Status:** accepted
- **Date:** 2026-10-01

## Context

The hive CLI released from `publish.yml` with `v*` semver tags picked by a
bump level. Hive Desktop released from the maintainer's machine with
`desktop-v*` tags and three channels (ADR release-channels). Two versions, two
tag prefixes, two triggers, and a site deploy that only the CLI ran. The
maintainer decided that every program ships together, under one version, on
one release line, cut from the maintainer's machine because the desktop's
signing keys live there.

The version has to stay valid semver: Go modules, GoReleaser, the CLI's update
check, and the desktop updater all parse it. Go also requires a `/vN` module
path suffix for a major version of 2 or more, so a version that starts with
the year, such as `v2026.930.1`, would leave
`go install github.com/colonyops/hive@latest` on the last v0 release.

## Decision

- **One version: `0.YYYYMMDD.N`**, tagged `v0.YYYYMMDD.N`. The date is UTC and
  N counts that day's releases from 0. It sorts above every CLI `v0.*` and
  desktop `0.*` version already published.
- **Promotion picks the version.** `release changelog promote` names it, and
  `release run` publishes the newest version that every program has an entry
  for and no release has published. Release notes that land after midnight
  still release under the version they were promoted as.
- **One release run.** `mise run release` on the maintainer's machine runs the
  gates, publishes the desktop to R2, pushes the `v*` tag, and dispatches
  `publish.yml`. The workflow builds the CLI with GoReleaser, creates the one
  GitHub release with every program's notes (`release changelog notes`), and
  deploys the site. The local run waits for it. `release cli tag` and the
  bump input are gone.
- **One release line.** A release is a bare `X.Y.Z`. It writes the stable,
  beta, and dev manifests, so a desktop install on any channel converges on it.

## Consequences

- This replaces the channel routing and cascade of ADR release-channels; the
  manifest layout stays. The rule of ADR
  desktop-github-releases-never-take-github-latest no longer applies: the one
  GitHub release per version is the CLI's release too, and it takes Latest.
  The `desktop-v*` tag of ADR github-tags-and-releases becomes `v*`.
- The CLI's GitHub release body is the promoted notes, not a commit log
  (ADR every-program-keeps-its-own-release-notes-and-promotes-them-under-one-version).
- The desktop's `updates.channel` setting still works, because every channel
  manifest carries the same release. Removing it from the app is follow-up
  work.
- The migration-order gate compares against the newest `v*` or `desktop-v*`
  tag.
