# Desktop GitHub releases never take GitHub Latest

- **Status:** superseded by [every-program-ships-under-one-shared-version](2026-10-01-every-program-ships-under-one-shared-version.md)
- **Date:** 2026-09-29

## Context

The desktop is moving into `colonyops/hive`, where it will share one GitHub
releases page with the hive CLI. The CLI's update check reads `releases/latest`
from that repo and compares the tag to its own version. GitHub keeps one Latest
release per repo.

ADR github-tags-and-releases made a stable desktop release Latest. On a shared
page that release would take Latest from the CLI, and every CLI install would
read a desktop tag as its newest version.

## Decision

`cmd/release` passes `--latest=false` on every GitHub release it creates:
stable, beta, and dev. dev and beta stay marked prerelease.

This replaces the "stable is Latest" rule in ADR github-tags-and-releases. The
rest of that decision stands.

The change lands before the move, so no desktop release is created on the
shared page with the old behavior.

## Consequences

- On `hay-kot/hive-desktop` the Latest badge stays on the last stable release
  cut before this change, and newer stable releases ship without it. Nothing
  reads the badge: downloads and the updater go through the R2 channel
  manifests.
- After the move, Latest belongs to the CLI.
- GitHub cannot unset Latest on a release. If a desktop release is ever marked
  Latest by hand, mark the newest CLI release Latest again with
  `gh release edit <tag> --latest`.
