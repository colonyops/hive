import type { SessionPullRequest } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

// Kept byte-identical to the `prlink` shell script this replaces: links already
// pasted into Slack read a certain way, and formatting that differs by which
// tool produced it is worse than either format.

function counts(pr: SessionPullRequest): string {
  return `(+${pr.additions}, -${pr.deletions})`
}

/** One link whose text carries the repository, title and diff size. */
export function markdownPullRequestLink(pr: SessionPullRequest, repo: string): string {
  return `[[${repo}] ${pr.title} \`${counts(pr)}\`](${pr.url})`
}
