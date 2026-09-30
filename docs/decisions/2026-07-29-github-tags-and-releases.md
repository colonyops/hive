# Publish GitHub tags and Releases as the source-side record of a desktop release

- **Status:** accepted; the "stable is Latest" rule superseded by [ADR desktop-github-releases-never-take-github-latest](2026-09-29-desktop-github-releases-never-take-github-latest.md)
- **Date:** 2026-07-29

## Context

Distribution runs entirely through R2 and channel manifests ([r2-manifest-distribution](2026-07-23-r2-manifest-distribution.md)): the app never reads GitHub, and the repo is private so it cannot serve public downloads. 0003 left the `desktop-v*` tag as the "source-side version anchor and changelog record," but the flow never delivered on the changelog half — the publisher created no tag, the runbook created a lightweight tag locally and told the operator not to push it, and no GitHub Release existed. So there was no per-version changelog anywhere, and the in-app "release notes" link (`wailsui.ReleaseURL`, which points at `releases/tag/desktop-v<version>`) resolved to nothing.

## Decision

A successful `cmd/release publish` records the release on GitHub as its final step, after the artifacts are uploaded and verified.

- **Push the tag, don't hand-tag.** The publisher creates the lightweight `desktop-v<version>` tag at the release commit and pushes it to origin. The manual `git tag` step is gone. The tag stays lightweight (the GitHub Release, not the tag, holds the notes) and keeps the existing namespace.
- **A GitHub Release per version, notes generated from the git range.** ~~`gh release create --generate-notes --notes-start-tag <previous desktop tag>` produces the PR/commit changelog since the previous desktop tag; a short header prepended via `--notes` states that downloads come from R2 and links the `releases/<version>/` prefix and `SHA256SUMS`. The previous tag is the greatest `desktop-v*` below this version **that is present on origin**, so the boundary is correct once tags are pushed and degrades to GitHub's own baseline for the bootstrap release whose predecessor was never pushed.~~ **Superseded by [release-notes-ship-inside-the-binary](2026-08-06-release-notes-ship-inside-the-binary.md):** the release body is now the version's committed changelog entry, still behind the same R2 download header. Generated notes cannot describe the binary that ships them, and the app needs the same text offline. The rest of this decision stands.
- **dev and beta are prereleases; ~~stable is Latest.~~** The channel drives `--prerelease` for dev and beta, so a dev build never sits above a shipped stable on the releases page. **Superseded in part by [desktop-github-releases-never-take-github-latest](2026-09-29-desktop-github-releases-never-take-github-latest.md):** every release, stable included, passes `--latest=false`.
- **No artifacts attached.** The Release carries notes only; R2 remains the sole artifact store (0003). The repo being private makes GitHub-hosted downloads a non-option anyway, but this holds regardless.
- **Last step, and independently recoverable.** The GitHub work runs after the irreversible R2 upload and verification, and is exposed as its own idempotent `release github <version>` subcommand. If it fails after the upload (a `gh` outage), the R2 release is already live and re-running `publish` would hit the immutability guard — so the recovery path re-records only the GitHub side. `gh` authentication is checked in `publish` preflight so a missing login aborts before any build.
- **Idempotent and conflict-aware.** A tag or Release already at the release commit is left untouched; a tag on origin pointing at a different commit is a hard error. `prepare` still rejects a pre-existing tag up front, so the conflict path is the recovery-run backstop, not the normal case.

## Consequences

- Every published version now has a changelog on GitHub, and the in-app release-notes link resolves to it.
- A release now needs an authenticated `gh` in addition to macOS and Docker; a local `--skip-upload` build records nothing on GitHub.
- Tags are pushed from now on; the two pre-existing unpushed tags stay local and un-released, which is why the previous-tag lookup tolerates a missing origin tag instead of assuming every predecessor is on origin.
- The Release is cosmetic to distribution — nothing reads it — so a failed or skipped GitHub step never affects what users download, only the changelog record.
