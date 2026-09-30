# Session git and pull-request status is computed in-app, not shelled out to hive or gh

- **Status:** accepted
- **Date:** 2026-08-14

## Context

The session status bar needs a session's branch, whether its checkout is dirty
or unpushed, and what its branch's pull request is doing. `colonyops/hive`
already computes all of this for its own TUI, and the obvious readings of "do
not write a second implementation" are to extend `hive session list --json` and
shell out to it, or to promote hive's `internal/core/git` and its `gh`-backed
PR plugin into that repo's `pkg/`.

Neither is necessary here: `cmd/vendorhive` already copies those packages into
`internal/hivecore/` with rewritten import paths, so `internal/hivecore/core/git`
is a direct dependency and `git.NewExecutor` is already constructed in
`internal/app/app.go`. Go's `internal/` rule never applies across the vendor
boundary.

The pull-request half is a different question, because hive's own answer has a
defect worth not inheriting: it runs one `gh pr view` per session and, on any
`gh` error, caches an empty result as "no pull request" for the full TTL. An
expired token therefore blanks the badge for minutes with no signal, and the
user reads "this branch has no PR" — a fact about their branch that is not true.

## Decision

Both halves are computed in this app.

Git goes through the vendored executor, behind a narrowed `sessionGit` seam in
`internal/app/dispatch` that exposes only the four read methods. Failures are
reported rather than assumed: hive's `CheckSessionRisk` treats an `IsClean`
error as dirty because a delete confirmation must over-warn, which is the wrong
default for a badge.

Pull requests go through the app's own GitHub client
(ADR owned-github-client) as one aliased GraphQL query, keyed by
`owner/repo` and `headRefName`, using the credential already in the keychain.
There is no `gh` dependency and no subprocess per session. The response
distinguishes four outcomes — found, none, disconnected, unsupported — and a
failed lookup is an error that caches nothing, so the next poll retries.

Reads are scoped to the session the bar is showing. A full git read is several
subprocesses, and the bar only ever displays one session.

## Consequences

- Nothing needs to land in `colonyops/hive` for this feature, and the vendor
  sync remains the only channel between the repos.
- Pull-request state costs one GraphQL request per repository/branch per cache
  TTL instead of one `gh` subprocess per session per refresh, and carries CI and
  review state that hive's plugin does not.
- `gh` need not be installed or authenticated for the bar to work; the GitHub
  connection the rest of the app already has is what backs it.
- Two implementations of "is this branch dirty" now exist across the two repos.
  That is accepted: the computation is four `git` invocations behind a seam
  pinned to the vendored interface, so an upstream change to it breaks this
  build rather than drifting silently.
- A session on a non-GitHub remote gets its git half and no pull-request
  lookup at all, rather than a failed one. Superseded for Gitea and Forgejo by
  ADR the-status-bar-routes-a-pull-request-lookup-by-the-remote-s-host, which
  routes the lookup by the remote's host instead of gating it on `github.com`.
