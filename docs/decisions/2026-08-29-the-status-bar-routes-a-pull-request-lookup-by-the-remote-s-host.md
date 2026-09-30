# The status bar routes a pull-request lookup by the remote's host

- **Status:** accepted
- **Date:** 2026-08-29

## Context

The session status bar's pull-request half was GitHub-only by construction: the
git half shells out to `git` and works anywhere, while the coordinates a lookup
needs were read through a `github.com` host gate, so a session on any other
remote reported `unsupported` and rendered no badge
(ADR session-git-and-pull-request-status-is-computed-in-app-not-shelled-out-to-hive-or-gh).

The Gitea connector since landed with the pieces the gate was waiting on: a
credential provider, an instance binding that records each connected account's
base URL, and an in-app REST client. What it does not bring is a second GraphQL
endpoint. GitHub answers the whole badge in one aliased query keyed by
`headRefName`; Gitea's API has no GraphQL at all, and its pull list filters by
base branch, state, labels, milestone and author — never by head. There is no
one request that answers "the pull request whose head is this branch".

`GET /repos/{owner}/{repo}/pulls/{base}/{head}` does answer it exactly, for a
closed or merged pull request as well as an open one. It is also the only
lookup that still finds a merged one: merging deletes the head branch and
rewrites the pull request's head ref to `refs/pull/N/head`, so a scan for the
branch name reports a freshly merged pull request as none — the state a session
is in exactly when its badge is worth reading.

## Decision

**The remote's host decides which forge is asked, and the connected accounts
decide which hosts a forge serves.**

`SessionGitStatus` and `SessionPullRequestKey` carry the host alongside
owner/repo. `internal/app/dispatch` no longer decides anything about forges: it
reads coordinates off the remote and answers empty only for a remote that names
no host, since `git.ExtractOwnerRepo` is host-agnostic and would otherwise read
the last two path segments of a local path.

A `forge` in `internal/app` is `serves(host)` plus a lookup returning the one
`dispatch.SessionPullRequest` view. GitHub serves `github.com`. Gitea serves
whatever hosts its connected accounts are bound to, compared without the port —
a remote carries the git transport's port, never the API's. A host no forge
serves stays `unsupported` and never `disconnected`: nothing identifies a host
as Gitea until an account bound to it says so, so "connect an account" is not
advice the app can honestly give about an arbitrary remote.

**The Gitea lookup asks by base and head, and scans only to cover another
base.** It reads the repository for its default branch, asks for the pull
request between that base and the branch, and falls back to one page of open
pull requests matched on head ref — which is what finds a pull request onto a
non-default base, and the only case that costs the extra request. Checks come
from the head commit's combined status; the review decision is folded out of
the review list, latest review per reviewer, and is read only for an open pull
request, since the bar paints a merged or closed one by its state.

## Consequences

- This supersedes the last consequence of
  ADR session-git-and-pull-request-status-is-computed-in-app-not-shelled-out-to-hive-or-gh:
  a session on a Gitea or Forgejo remote now gets the same badge as a GitHub
  one, and only a remote on neither forge gets its git half alone.
- A Gitea badge costs two to four REST requests per cache TTL where GitHub's
  costs one GraphQL request. That is a self-hosted instance behind a five-minute
  cache, so it is not a budget worth optimizing further.
- A pull request onto a non-default base is found only while it is open. Once
  merged, its head ref is gone and the by-base-and-head lookup asks the wrong
  base, so the badge reports none.
- Owner and repo are now populated for any hosted remote, not only a GitHub
  one. The Tasks overlay reads the same pair as its `hc` repo key, which is
  host-agnostic upstream, so a Gitea session's tasks scope where they always
  should have.
- GitHub Enterprise remains unsupported: an instance is reached at its own host
  and nothing in the app configures one. It would arrive as a third `forge`,
  not as a change to this seam.
