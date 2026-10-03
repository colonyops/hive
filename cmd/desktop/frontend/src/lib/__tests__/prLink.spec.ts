import { describe, expect, it } from 'vitest'
import { markdownPullRequestLink } from '../prLink'
import type { SessionPullRequest } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

function pullRequest(overrides: Partial<SessionPullRequest> = {}): SessionPullRequest {
  return {
    status: 'found',
    number: 315,
    title: 'Give a session an optional status bar',
    state: 'OPEN',
    isDraft: false,
    url: 'https://github.com/hay-kot/hive-desktop/pull/315',
    reviewDecision: '',
    checks: '',
    additions: 420,
    deletions: 37,
    ...overrides,
  } as SessionPullRequest
}

// The contract with the `prlink` shell script this replaces. Links already
// pasted into Slack and chat threads read a certain way, so drifting from it —
// even by a space — makes the app's links visibly different from those.
describe('pull request links', () => {
  it('renders the Markdown form the shell script produced', () => {
    expect(markdownPullRequestLink(pullRequest(), 'hive-desktop')).toBe(
      '[[hive-desktop] Give a session an optional status bar `(+420, -37)`](https://github.com/hay-kot/hive-desktop/pull/315)',
    )
  })

  // A brand new pull request has no diff yet, and "(+0, -0)" is the honest
  // rendering of that rather than something to suppress.
  it('renders zero counts rather than dropping them', () => {
    expect(markdownPullRequestLink(pullRequest({ additions: 0, deletions: 0 }), 'site')).toContain('`(+0, -0)`')
  })
})
