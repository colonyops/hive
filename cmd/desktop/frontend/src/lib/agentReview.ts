import { diffSpan, hasDiffLine, type DiffRow, type DiffSide } from './diffReview'
import type { ChangesComment } from '../stores/useChangesReview'
import type {
  DiffReviewCommentWire,
  DiffReviewWire,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type AgentSeverity = 'issue' | 'suggestion' | 'note'

/** An agent's comment with the review it belongs to, which is where its author is. */
export interface AgentComment extends Omit<DiffReviewCommentWire, 'startSide' | 'endSide'> {
  startSide: DiffSide
  endSide: DiffSide
  reviewId: string
  author: string
  /** The coding agent, for its mark. Empty when the review did not say. */
  agent: string
}

export function agentComments(reviews: readonly DiffReviewWire[]): AgentComment[] {
  return reviews.flatMap((review) =>
    (review.comments ?? []).map((comment) => ({
      ...comment,
      startSide: (comment.startSide || 'additions') as DiffSide,
      endSide: (comment.endSide || 'additions') as DiffSide,
      reviewId: review.id,
      author: review.author,
      agent: review.agent,
    })),
  )
}

/**
 * The order the walkthrough takes: the files as the rail lists them, a file's own
 * comments before its rows, rows by line. A file the rail does not list goes
 * last, so a comment on it is still reachable.
 */
export function walkOrder(comments: readonly AgentComment[], files: readonly string[]): AgentComment[] {
  const rank = new Map(files.map((path, index) => [path, index]))
  const fileRank = (path: string) => rank.get(path) ?? files.length
  return [...comments].sort(
    (a, b) =>
      fileRank(a.path) - fileRank(b.path) ||
      a.path.localeCompare(b.path) ||
      Number(b.kind === 'file') - Number(a.kind === 'file') ||
      a.startLine - b.startLine ||
      a.endLine - b.endLine,
  )
}

/**
 * Whether the rows under a comment no longer read as the agent quoted them:
 * the session's agent edited them, or they left the diff. A file comment has
 * no rows to move.
 */
export function isAgentCommentStale(comment: AgentComment, rows: readonly DiffRow[]): boolean {
  if (comment.kind === 'file') return false
  const span = diffSpan(rows, {
    start: comment.startLine,
    side: comment.startSide,
    end: comment.endLine,
    endSide: comment.endSide,
  })
  return !span || span.quote !== comment.quote
}

export function severityCounts(comments: readonly AgentComment[]): Record<AgentSeverity, number> {
  const counts: Record<AgentSeverity, number> = { issue: 0, suggestion: 0, note: 0 }
  for (const comment of comments) {
    const severity = comment.severity as AgentSeverity
    if (severity in counts) counts[severity]++
  }
  return counts
}

/** An agent comment and the row it is drawn under: a file comment sits on the file's first row. */
export interface AnchoredAgentComment {
  comment: AgentComment
  side: DiffSide
  line: number
}

/**
 * Where a comment is drawn in a file's diff, or null when it cannot be: its
 * last row left the diff, or the file has no rows. A file comment goes on the
 * first row, at the top of the file.
 */
export function anchorAgentComment(comment: AgentComment, rows: readonly DiffRow[]): AnchoredAgentComment | null {
  if (comment.kind === 'file') {
    const first = rows[0]
    if (!first) return null
    return first.type === 'deletion'
      ? { comment, side: 'deletions', line: first.oldLine }
      : { comment, side: 'additions', line: first.newLine }
  }
  return hasDiffLine(rows, comment.endLine, comment.endSide)
    ? { comment, side: comment.endSide, line: comment.endLine }
    : null
}

/**
 * An agent comment as Send delivers it beside the user's own: in the comparison
 * the agent wrote it in, with the author and severity leading its text.
 */
export function agentFeedback(comment: AgentComment): Omit<ChangesComment, 'id'> {
  return {
    ...(comment.kind === 'file' ? { kind: 'file' as const } : {}),
    path: comment.path,
    startSide: comment.startSide,
    startLine: comment.startLine,
    endSide: comment.endSide,
    endLine: comment.endLine,
    quote: comment.quote,
    base: 'branch',
    version: '',
    text: `${comment.author} (${comment.severity}): ${comment.text}`,
    from: comment.author,
  }
}
