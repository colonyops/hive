---
name: desktop-release
description: Cut the next Hive Desktop dev, beta, or stable release. Use when asked to release or publish the desktop app, or when explicitly invoked with a release channel such as `/desktop-release dev`.
compatibility: Requires git, Go, mise, GitHub CLI authentication, a running Docker (Linux binaries build in a container), and the release secrets in the repo-root `.env`.
disable-model-invocation: true
---

# Release Hive Desktop

Cut a release through the local publisher. `mise` loads release credentials
automatically; never read `.env`, inspect secret values, or invoke the release
CLI's `publish` command outside `mise`. Publishing is local-only — every
platform (macOS plus both Linux architectures) is built on one machine, so the
release needs macOS and a running Docker together (ADR linux-tarball-distribution). There is no
CI publishing workflow.

Publishing records the release on GitHub as its final step: it pushes the
`desktop-v<version>` tag and creates a GitHub Release whose body is the
version's committed release notes (dev and beta marked prerelease). Downloads
still come from R2 (ADR r2-manifest-distribution); the release attaches no artifacts. That step
is idempotent — `go run ./cmd/tools/release github <version>` re-records a release
whose GitHub step failed after the upload.

**Only a stable release needs a changelog entry.** Notes are embedded in the
binary, so an entry written after the build would describe a release that cannot
display it (ADR release-notes-ship-inside-the-binary) — which is why `prepare`
and `publish` refuse a stable version with none. A dev or beta release needs no
changelog work at all: it publishes the fragments in
`cmd/desktop/releasenotes/changelog/unreleased/` as they stand. Step 4 below covers what to do when `prepare` reports an entry
missing.

## Arguments

Accept one of these channels:

- `dev`
- `beta`
- `stable`

An explicit version may follow the channel, for example `dev 0.2.0-dev.1`.
Reject missing or unknown channels instead of guessing.

## Procedure

1. Resolve the repository root with `git rev-parse --show-toplevel`, change to
   it, and run every command below from there. Read `docs/distribution.md`.
2. Run `git fetch origin main --tags --prune`.
3. Require all of the following:
   - the working tree is clean;
   - the current branch is `main`;
   - local `main` is identical to `origin/main` (neither ahead nor behind);
   - `gh auth status` succeeds.

   Stop and explain the mismatch if any check fails. Do not stash, reset, merge,
   pull, switch branches, or discard work automatically.
4. Select and validate the candidate with the Go release CLI. `prepare` also
   runs `cmd/desktop/scripts/check-migration-order.sh`, which rejects gaps and any change to
   a SQLite migration already present in the latest reachable desktop release
   tag. If no explicit version was supplied, run:

   ```bash
   go run ./cmd/tools/release prepare <channel>
   ```

   Otherwise run:

   ```bash
   go run ./cmd/tools/release prepare <channel> <version>
   ```

   For a **stable** release, `prepare` also refuses a version with no changelog
   entry, naming the file it wants. **This is not recoverable inside the release
   run**: the entry has to be committed on `main` before publishing, and step 3
   requires a clean tree identical to `origin/main`, so it cannot be written
   here. Stop and tell the operator to run `/release-prep <version>`.

   The `release-prep` skill promotes the accumulated fragments, curates the
   stable entry, validates it, and runs the repository command that creates the
   branch, commit, push, and pull request. Never create those by hand. Restart
   this procedure from step 2 after the pull request merges.

   Dev and beta releases never reach this step: they are not gated, and they
   publish the draft as it stands.

5. Use the candidate and manifest state printed by `prepare`. It reads all live
   stable, beta, and dev manifests (treating 404 as an empty channel), includes
   both manifests and `desktop-v*` tags when selecting the next version, checks
   the channel-specific format and dev → beta → stable advancement across every
   affected manifest, and rejects a tag
   that already exists locally or on `origin`. Stop if it reports any error.
   Never choose from repository tags alone: the R2 history predates this
   repository and may contain a newer version than any local tag.
6. Show the candidate version, channel, exact commit SHA and subject, and
   affected manifests (`dev`; `beta+dev`; or `stable+beta+dev`). Ask for an
   explicit confirmation before doing anything that publishes. Prefer the
   harness's structured confirmation UI when available; otherwise ask in plain
   text and wait. The confirmation must make clear that the local workflow will
   build, sign, notarize, and upload a public release. A typed alternate version
   is acceptable; revalidate it. Stop on cancellation.
7. After confirmation, run the same local gates used before pushes:

   ```bash
   mise run check
   mise run desktop:frontend:test
   ```

   Stop on the first failure. Verify the worktree is still clean and `HEAD`
   still equals `origin/main` afterward.
8. Publish through `mise`, which loads the credentials without exposing them:

   ```bash
   mise run desktop:release:publish -- <version>
   ```

   Do not read `.env`, print credential environment variables, or call
   `go run ./cmd/tools/release publish` directly. The publisher verifies every affected
   live manifest and downloads the public artifact to check its size and SHA-256
   before it succeeds.

   R2 operations have bounded retries. If they are exhausted after packaging has
   completed, keep `cmd/desktop/bin` intact and resume the same version:

   ```bash
   mise run desktop:release:publish -- <version> --resume
   ```

   Resume skips builds, signing, and Apple submissions. It
   re-verifies the local artifacts, reuses only byte-identical R2 objects,
   uploads missing objects, and finishes partial manifest writes before the
   normal live verification and GitHub step. Stop if resume reports a local or
   remote mismatch. Never use `--force` for recovery and never invent a
   replacement version for a partial upload.
9. Publishing finishes by recording the release on GitHub itself — pushing the
   `desktop-v<version>` tag and creating its GitHub Release. Do not tag or push
   by hand. If only that final step fails (e.g. a `gh` outage), the R2 release is
   already live and verified; re-run just the idempotent GitHub step:

    ```bash
    go run ./cmd/tools/release github <version>
    ```

    Report the version, pushed tag and its GitHub Release URL, affected channel
    manifests, and artifact URL prefix
    (`https://dl.hivedesktop.com/desktop/releases/<version>/`).

## Version selection rules

The Go release CLI uses the highest numeric base version found across both
`desktop-v*` tags and the live stable, beta, and dev manifests. Including the
manifests is essential because the R2 release history predates this repository;
for example, a `0.1.1-dev.1` tag cannot update clients already on
`0.1.8-dev.1`.

- another release in the same channel increments its prerelease number;
- beta promotes the current dev base to `-beta.1`;
- stable promotes the current prerelease base to the bare version;
- a dev release after beta/stable advances the patch and starts at `-dev.1`;
- a beta or stable release after stable advances the patch;
- when no release tags exist, the base is `0.1.0`.

The confirmation step is the opportunity to choose a minor or major bump
instead of the computed patch-oriented default.
