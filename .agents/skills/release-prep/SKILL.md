---
name: release-prep
description: Curate every program's accumulated changelog fragments into its entry for the next release, then create the release-notes pull request. Use when explicitly asked to prepare release notes or invoked as `/release-prep [version]`.
compatibility: Requires git, Go, mise, GitHub CLI authentication, and access to the live release manifests used for version selection.
argument-hint: "[patch|minor|major|version]"
disable-model-invocation: true
---

# Prepare release notes

Prepare the changelog entries that must land before a release. Every
release ships the hive CLI and Hive Desktop under one version, so promotion
writes one entry per program. This command edits product copy, then delegates
the branch, commit, push, and pull request to the repository's release tooling.

The version this promotes is the version the release publishes: `mise run
release` reads it from the entries.

## Arguments

Accept no argument, one bump level (`patch`, `minor`, or `major`), or one
explicit version such as `0.60.0`. No argument means `minor`. The release tool
bumps the newest version across every tag and live manifest by that level.
Reject a prerelease, a `v` prefix, and extra arguments.

## 1. Load the project rules

Resolve the repository root with `git rev-parse --show-toplevel`, change to it,
and run every command from there. Read these files before editing:

- [`../release-notes/SKILL.md`](../release-notes/SKILL.md) for product-copy rules;
- `docs/distribution.md` for the current release contract.

The repository command `mise run changelog:pr` is the authority on the branch,
commit, push, and pull request. Do not use `pr-create-auto`, create the branch by
hand, or write a separate commit.

## 2. Validate the starting state

Run:

```bash
git fetch origin main --tags --prune
git status --porcelain=v1 --untracked-files=all
git branch --show-current
git rev-parse HEAD
git rev-parse origin/main
gh auth status
```

Require `main`, `HEAD` equal to `origin/main`, and successful GitHub
authentication. Do not pull, reset, stash, switch branches, or discard work to
make the checks pass.

Two worktree states are valid:

1. **Clean:** run `mise run changelog:promote`, adding `-- --bump <level>` for
   a bump level or `-- <version>` for an explicit version.
2. **Already promoted:** continue without promoting again when the only changes
   are one untracked `<version>.md` in each of `cmd/hive/releasenotes/changelog/`
   and `cmd/desktop/releasenotes/changelog/`, all for the same version, and
   deleted files under their `unreleased/` directories.

For an already promoted tree, require the entry filename to match an explicit
version argument. Any other dirty state is unrelated work. Stop and report it.

Promotion writes an empty `summary`, combines each program's fragments into
its entry, and deletes those fragments. A program with no fragments still gets
an entry, with an empty body. Never create or rename a versioned entry by hand.

## 3. Curate the entries

Curate each program's entry on its own: its readers are that program's users.
Read the promoted entry, its deleted source fragments from `HEAD`, and recent
committed entries for voice and structure. Read the commit history since the
last release when a note needs verification. Inspect a focused diff only
when the history does not establish the user-visible behavior.

Edit each promoted entry as one release, not as a list of pull requests:

- write one quoted `summary` sentence that names the release's main user-facing
  outcomes;
- combine bullets that describe the same feature or surface;
- remove notes for work reverted before this release;
- update stale bullets so they describe the behavior that will ship;
- order bullets by user value within `Added`, `Changed`, and `Fixed`;
- remove empty sections and keep the remaining sections in that order;
- preserve specific UI labels, shortcuts, config keys, and platform differences;
- do not mention implementation names, commits, issues, pull requests, or ADRs;
- do not invent claims that the fragments or repository history do not support.

Follow the release-notes skill's product-copy rules. The summary and each bullet
must describe what the user gets in the release. Do not write a work log
or a release-process summary.

An entry with an empty body still needs a `summary`. Say in one sentence that
the release has no user-facing changes to that program; do not invent any.

Read each complete entry again after editing. Check that the summary covers the
body, near-duplicate bullets are gone, and every sentence still describes the
current product.

## 4. Verify and create the pull request

Run the focused parser tests and the release tool's dry run:

```bash
go test ./cmd/hive/releasenotes/... ./cmd/desktop/releasenotes/...
mise run changelog:pr -- --dry-run
```

Run `git diff --check`, then read each complete entry and the deleted-fragment list
one final time. Fix all failures before continuing. Then run:

```bash
mise run changelog:pr
```

This command creates the fixed release-notes branch, commits only the promoted
entries and deleted fragments, pushes the branch, and opens the pull request. The
git hooks run the repository checks. Do not duplicate those steps with manual
git or `gh` commands.

If the command says it committed and pushed but `gh pr create` failed, the
promotion is already safe on the release-notes branch. Follow the recovery
command from the error exactly and do not promote or commit again. For any
other failure, stop without rewriting history or changing branches to hide the
problem.

Report the version, summary, branch, commit, pull request URL, and tests that
ran.
