# Every program ships under one shared version

- **Status:** accepted
- **Date:** 2026-10-01

## Context

Every program in the repository ships together, under one version, cut from
the maintainer's machine because the desktop's signing keys live there.

The version must be valid semver: Go modules, GoReleaser, the CLI's update
check, and the desktop updater all parse it. Go requires a `/vN` module path
suffix for a major version of 2 or more, which rules out a calendar version
such as `v2026.930.1`: `go install github.com/colonyops/hive@latest` would
stay on the last v0 release.

## Decision

- **One semver version, bumped in lockstep.** Every program takes the same
  version, tagged `v<version>`. The next version bumps the newest one across
  every `v*` and `desktop-v*` tag and every live desktop manifest, by minor
  unless the release asks for patch or major.
- **Promotion picks the version.** `release changelog promote` names it, and
  `release run` publishes the newest version that every program has an entry
  for and no release has published.
- **One release run.** `mise run release` runs the gates, publishes the
  desktop to R2, pushes the `v*` tag, and dispatches `publish.yml`, then
  waits for it. The workflow builds the CLI with GoReleaser and the
  `hive-drive-server` image, creates the one GitHub release with every
  program's notes (`release changelog notes`), and deploys the site.
- **One release line.** A release is a bare `X.Y.Z`. It writes the stable,
  beta, and dev desktop manifests, so an install on any channel converges on
  it.

## Consequences

- The one GitHub release per version is the CLI's release too, and it takes
  GitHub Latest.
- A release bumps every program, including one it does not change
  (ADR every-program-keeps-its-own-release-notes-and-promotes-them-under-one-version).
- The desktop's `updates.channel` setting still works, because every channel
  manifest carries the same release.
