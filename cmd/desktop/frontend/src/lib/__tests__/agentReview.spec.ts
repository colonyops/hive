import { describe, expect, it } from 'vitest'
import { agentComments, anchorAgentComment, isAgentCommentStale, walkOrder, type AgentComment } from '../agentReview'
import { diffRows } from '../diffReview'

const rows = diffRows('@@ -1,2 +1,3 @@\n a\n-b\n+c\n+d\n')

function comment(extra: Partial<AgentComment>): AgentComment {
  return {
    id: 'x',
    kind: '',
    path: 'a.go',
    startSide: 'additions',
    startLine: 2,
    endSide: 'additions',
    endLine: 3,
    quote: '+c\n+d',
    severity: 'issue',
    text: 't',
    reviewId: 'r',
    author: 'claude',
    agent: 'claude',
    ...extra,
  }
}

describe('agentReview', () => {
  it('reads the side of a comment with none as additions', () => {
    const [first] = agentComments([
      {
        id: 'r',
        author: 'a',
        agent: '',
        head: '',
        summary: '',
        createdAt: '',
        comments: [{ ...comment({}), startSide: '', endSide: '' }],
      },
    ])
    expect(first).toMatchObject({ startSide: 'additions', endSide: 'additions', reviewId: 'r', author: 'a' })
  })

  it('orders by the rail, the file comment first, then by line, and an unlisted file last', () => {
    const order = walkOrder(
      [
        comment({ id: 'late', startLine: 9 }),
        comment({ id: 'unlisted', path: 'z.go' }),
        comment({ id: 'file', kind: 'file', startLine: 0 }),
        comment({ id: 'b', path: 'b.go' }),
        comment({ id: 'early', startLine: 1 }),
      ],
      ['b.go', 'a.go'],
    )
    expect(order.map((c) => c.id)).toEqual(['b', 'file', 'early', 'late', 'unlisted'])
  })

  it('is stale when the rows no longer read as quoted, or are gone', () => {
    expect(isAgentCommentStale(comment({}), rows)).toBe(false)
    expect(isAgentCommentStale(comment({ quote: '+c\n+e' }), rows)).toBe(true)
    expect(isAgentCommentStale(comment({ endLine: 40 }), rows)).toBe(true)
    expect(isAgentCommentStale(comment({ kind: 'file', quote: '' }), rows)).toBe(false)
  })

  it('anchors a file comment on the first row and a row comment on its last', () => {
    expect(anchorAgentComment(comment({}), rows)).toMatchObject({ side: 'additions', line: 3 })
    expect(anchorAgentComment(comment({ kind: 'file' }), rows)).toMatchObject({ side: 'additions', line: 1 })
    expect(anchorAgentComment(comment({ endLine: 40 }), rows)).toBeNull()
    expect(anchorAgentComment(comment({ kind: 'file' }), [])).toBeNull()
  })
})
