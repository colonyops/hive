# The status bar routes a pull-request lookup by the remote's host

- **Status:** accepted
- **Date:** 2026-08-29

## Context

The session status bar's pull-request lookup
(ADR session-git-and-pull-request-status-is-computed-in-app-not-shelled-out-to-hive-or-gh)
must work on Gitea and Forgejo remotes as well as GitHub. Gitea has no GraphQL,
and its pull list cannot filter by head branch. `GET
/repos/{owner}/{repo}/pulls/{base}/{head}` answers exactly, including for a
merged pull request, whose head ref Gitea rewrites to `refs/pull/N/head` so a
scan by branch name misses it.

## Decision

**The remote's host decides which forge is asked, and the connected accounts
decide which hosts a forge serves.**

`gitstatus.RemoteCoordinates` reads host, owner, and repo off the remote, and
`SessionPullRequestKey` carries all three plus the branch. A remote that names
no host (a local path) yields nothing.

A `forge` in `cmd/desktop/internal/app` is `serves(host)` plus a lookup that
returns the one `SessionPullRequest` view. GitHub serves `github.com`. Gitea
serves whatever hosts its connected accounts are bound to, compared without the
port, since a remote carries the git transport's port, not the API's. A host no
forge serves is `unsupported`, never `disconnected`: nothing identifies a host
as Gitea until an account bound to it exists.

**The Gitea lookup asks by base and head, and scans only to cover another
base** (`giteaclient.PullRequestForBranch`). It reads the repository's default
branch, asks for the pull request between that base and the branch, and falls
back to one page of open pull requests matched on head ref. Checks come from the
head commit's combined status. The review decision is folded from the review
list (latest per reviewer) and read only for an open pull request.

## Consequences

- A pull request onto a non-default base is found only while it is open. Once
  merged its head ref is gone and the base-and-head lookup asks the wrong base,
  so the badge reports none.
- A Gitea badge costs two to four REST requests per cache TTL.
- GitHub Enterprise is unsupported. It would arrive as another `forge`, not as a
  change to this seam.
