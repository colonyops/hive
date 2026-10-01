---
name: desktop-release-prep
description: Curate Hive Desktop's accumulated changelog fragments into the next stable release entry, then create the release-notes pull request. Use when explicitly asked to prepare stable release notes or invoked as `/desktop-release-prep [version]`.
compatibility: Requires git, Go, mise, GitHub CLI authentication, and access to the live release manifests used for version selection.
argument-hint: "[stable|version]"
disable-model-invocation: true
---

# Prepare stable release notes

Prepare the changelog entry that must land before a stable Hive Desktop release.
This command edits product copy, then delegates the branch, commit, push, and pull
request to the repository's release tooling.

Do not use this for dev or beta releases. They publish the accumulated draft as
it stands and need no release-notes pull request.

## Arguments

Accept either no argument, `stable`, or one explicit stable version such as
`0.5.3`. No argument means `stable`, which asks the release tool to select the
next version from tags and live manifests. Reject prerelease versions and extra
arguments.

## 1. Load the project rules

Resolve the repository root with `git rev-parse --show-toplevel`, change to it,
and run every command from there. Read these files before editing:

- [`../desktop-release-notes/SKILL.md`](../desktop-release-notes/SKILL.md) for product-copy rules;
- `docs/distribution.md` for the current release contract.

The repository command `mise run desktop:changelog:pr` is the authority on the branch,
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

1. **Clean:** run `mise run desktop:changelog:promote -- <stable|version>`.
2. **Already promoted:** continue without promoting again when the only changes
   are one untracked `cmd/desktop/releasenotes/changelog/<version>.md` file and
   deleted files under `cmd/desktop/releasenotes/changelog/unreleased/`.

For an already promoted tree, require the entry filename to match an explicit
version argument. Any other dirty state is unrelated work. Stop and report it.

Promotion writes an empty `summary`, combines the fragments into one entry, and
deletes those fragments. Never create or rename the versioned entry by hand.

## 3. Curate the entry

Read the promoted entry, its deleted source fragments from `HEAD`, and recent
committed entries for voice and structure. Read the commit history since the
last stable release when a note needs verification. Inspect a focused diff only
when the history does not establish the user-visible behavior.

Edit the promoted entry as one release, not as a list of pull requests:

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
must describe what the user gets in the stable release. Do not write a work log
or a release-process summary.

Read the complete entry again after editing. Check that the summary covers the
body, near-duplicate bullets are gone, and every sentence still describes the
current product.

## 4. Verify and create the pull request

Run the focused parser tests and the release tool's dry run:

```bash
go test ./cmd/desktop/releasenotes/...
mise run desktop:changelog:pr -- --dry-run
```

Run `git diff --check`, then read the complete entry and deleted-fragment list
one final time. Fix all failures before continuing. Then run:

```bash
mise run desktop:changelog:pr
```

This command creates the fixed release-notes branch, commits only the promoted
entry and deleted fragments, pushes the branch, and opens the pull request. The
git hooks run the repository checks. Do not duplicate those steps with manual
git or `gh` commands.

If the command says it committed and pushed but `gh pr create` failed, the
promotion is already safe on the release-notes branch. Follow the recovery
command from the error exactly and do not promote or commit again. For any
other failure, stop without rewriting history or changing branches to hide the
problem.

Report the version, summary, branch, commit, pull request URL, and tests that
ran.
