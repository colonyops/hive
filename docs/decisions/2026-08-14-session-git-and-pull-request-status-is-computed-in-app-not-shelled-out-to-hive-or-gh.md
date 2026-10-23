# Session git and pull-request status is computed in-app, not shelled out to hive or gh

- **Status:** accepted
- **Date:** 2026-08-14

## Context

The session status bar shows a session's branch, whether its checkout is dirty
or unpushed, and what its branch's pull request is doing. Shelling out to
`hive session list --json` or `gh pr view` would work, but `gh`'s error
handling caches an empty result as "no pull request", so an expired token
blanks the badge with no signal and the user reads a false fact about their
branch.

## Decision

Both halves are computed in-process.

**Git** goes through the engine's `internal/hive/gitstatus`, which reads
branch, dirty, unpushed, and diff stats behind a narrow `Git` interface. A
failed read is reported in `Status.Err`, never assumed: a failed `IsClean`
leaves `Dirty` false, because a badge claiming uncommitted work on a git that
never ran is a lie the user acts on. Reads are per session
(`SessionService.SessionGitStatus`), since the bar shows one session.

**Pull requests** go through the app's own forge clients
(`cmd/desktop/internal/app/session_pull_requests.go`), with no `gh` and no
subprocess. On GitHub that is one aliased GraphQL query over the keychain
credential (ADR owned-github-client); other hosts are routed by
ADR the-status-bar-routes-a-pull-request-lookup-by-the-remote-s-host. The result
is one of `found`, `none`, `disconnected`, `unsupported`. A failed lookup is an
error and caches nothing, so the next poll retries. Results are cached for five
minutes; the git half polls more often.

## Consequences

- `gh` need not be installed or authenticated for the bar to work.
- Keep the four outcomes apart. Rendering "no pull request" for a failed lookup
  or a disconnected account states a different, wrong fact.
