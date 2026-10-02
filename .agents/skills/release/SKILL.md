---
name: release
description: Cut the next release of every program -- the hive CLI and Hive Desktop under one version. Use when asked to release or publish hive, the CLI, or the desktop app, or when explicitly invoked as `/release`.
compatibility: Requires git, Go, mise, GitHub CLI authentication, a running Docker (Linux binaries build in a container), and the release secrets in the repo-root `.env`.
disable-model-invocation: true
---

# Release every program

Every release ships the hive CLI and Hive Desktop together, under one semver
version tagged `v<version>` (ADR
every-program-ships-under-one-shared-version). The run starts on this
machine because the desktop's signing keys live here. `mise` loads release
credentials automatically; never read `.env`, inspect secret values, or invoke
the release CLI's `publish` command outside `mise`.

The run publishes the desktop first: it builds every platform here (macOS plus
both Linux architectures, so it needs macOS and a running Docker together),
uploads to R2, and writes the stable, beta, and dev manifests. It then pushes
the `v<version>` tag and dispatches `publish.yml`, which builds the CLI with
GoReleaser, creates the one GitHub release with every program's notes,
publishes the Homebrew cask, and deploys the site. The run waits for the
workflow. That last step is idempotent -- `go run ./cmd/tools/release github
<version>` re-runs it for a release whose GitHub step failed after the upload.

**The release notes pick the version.** Each program's notes are embedded in
its binary, so an entry written after the build would describe a release that
cannot display it (ADR release-notes-ship-inside-the-binary). The release
publishes the newest version that every program has a promoted entry for and
that no release has published. Step 4 covers what to do when there is none.

## Arguments

Accept no arguments. Reject a channel or a version: the promoted entries name
the version, and there is one release line.

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
4. Select and validate the candidate with the Go release CLI:

   ```bash
   go run ./cmd/tools/release prepare
   ```

   `prepare` takes the version from the promoted entries and refuses one that
   does not advance every `v*` and `desktop-v*` tag and every live manifest,
   that lacks an entry with a summary for any program, or whose tag exists
   locally or on `origin`. It also runs
   `cmd/desktop/scripts/check-migration-order.sh`, which rejects gaps and any
   change to a SQLite migration already present in the latest reachable release
   tag.

   If it reports that no promoted release notes are newer than the last
   release, **this is not recoverable inside the release run**: the entries have
   to be committed on `main` before publishing, and step 3 requires a clean tree
   identical to `origin/main`, so they cannot be written here. Stop and tell the
   operator to run `/release-prep`. The `release-prep` skill promotes the
   accumulated fragments, curates each program's entry, and runs the repository
   command that creates the branch, commit, push, and pull request. Restart this
   procedure from step 2 after the pull request merges.

5. Use the candidate, tag, commit, and manifest state printed by `prepare`.
   Stop if it reports any error. Never choose a version from repository tags
   alone: the R2 history predates this repository and may hold a newer version
   than any local tag.
6. Show the candidate version, tag, exact commit SHA and subject, and the
   current stable, beta, and dev manifests. Ask for an explicit confirmation
   before doing anything that publishes. Prefer the harness's structured
   confirmation UI when available; otherwise ask in plain text and wait. The
   confirmation must make clear that the run will build, sign, notarize, and
   upload a public desktop release, push the tag, and publish the CLI. Stop on
   cancellation.
7. After confirmation, run the same local gates used before pushes:

   ```bash
   mise run check
   mise run desktop:frontend:test
   ```

   Stop on the first failure. Verify the worktree is still clean and `HEAD`
   still equals `origin/main` afterward.
8. Publish through `mise`, which loads the credentials without exposing them:

   ```bash
   mise run release:publish -- <version>
   ```

   Do not read `.env`, print credential environment variables, or call
   `go run ./cmd/tools/release publish` directly. The publisher verifies every
   live manifest and downloads the public artifact to check its size and
   SHA-256 before it moves on to the tag.

   R2 operations have bounded retries. If they are exhausted after packaging has
   completed, keep `cmd/desktop/bin` intact and resume the same version:

   ```bash
   mise run release:publish -- <version> --resume
   ```

   Resume skips builds, signing, and Apple submissions. It re-verifies the local
   artifacts, reuses only byte-identical R2 objects, uploads missing objects,
   and finishes partial manifest writes before the normal live verification and
   GitHub step. Stop if resume reports a local or remote mismatch. Never use
   `--force` for recovery and never invent a replacement version for a partial
   upload.
9. Publishing finishes by pushing the `v<version>` tag and dispatching
   `publish.yml`, then watching the run. Do not tag or push by hand. If only
   that step fails (a `gh` outage, a failed workflow run), the desktop release is
   already live and verified. Fix the cause, then re-run just the idempotent
   GitHub step:

   ```bash
   go run ./cmd/tools/release github <version>
   ```

   Report the version, the pushed tag and its GitHub release URL, the workflow
   run URL, and the desktop artifact URL prefix
   (`https://dl.hivedesktop.com/desktop/releases/<version>/`).

## Version rules

- A version is semver `X.Y.Z`, bumped in lockstep for every program. Promotion
  bumps the newest published version by minor unless asked for patch or major.
  `go run ./cmd/tools/release next` prints the version it would take.
- A major bump to 2 or more needs a `/vN` module path for the CLI's Go module.
  Never bump past 1 without that change.
- A release must advance every `v*` tag, every `desktop-v*` tag, and every live
  manifest. Including the manifests matters because the R2 history predates
  this repository.
