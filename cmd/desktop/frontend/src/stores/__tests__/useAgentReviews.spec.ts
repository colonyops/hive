import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgentReviews } from '../useAgentReviews'
import { resetStores } from '../defineStore'

const mocks = vi.hoisted(() => ({
  DiffReviews: vi.fn(),
  DismissDiffComment: vi.fn(),
  DismissDiffReview: vi.fn(),
  On: vi.fn(() => () => {}),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice', () => ({
  DiffReviews: mocks.DiffReviews,
  DismissDiffComment: mocks.DismissDiffComment,
  DismissDiffReview: mocks.DismissDiffReview,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

const review = (id: string, comments: string[]) => ({
  id,
  author: 'claude',
  agent: 'claude',
  head: 'abc',
  summary: '',
  createdAt: '2026-11-04T00:00:00Z',
  comments: comments.map((commentId) => ({
    id: commentId,
    kind: '',
    path: 'a.go',
    startSide: 'additions',
    startLine: 1,
    endSide: 'additions',
    endLine: 1,
    quote: '+x',
    severity: 'note',
    text: commentId,
  })),
})

function fire(): void {
  const handler = (mocks.On.mock.calls as unknown as [string, (event: { data: unknown }) => void][]).find(
    ([name]) => name === 'diff-reviews:updated',
  )?.[1]
  handler?.({ data: 'changed' })
}

describe('useAgentReviews', () => {
  beforeEach(() => {
    resetStores()
    vi.clearAllMocks()
    mocks.DismissDiffComment.mockResolvedValue(undefined)
  })

  it('reads every watched session again when Go says a review changed, and stops when unwatched', async () => {
    mocks.DiffReviews.mockResolvedValue([review('r1', ['c1'])])
    const store = useAgentReviews()
    const stop = store.watch('s1')
    await flushPromises()
    expect(store.reviews('s1')).toHaveLength(1)

    fire()
    await flushPromises()
    expect(mocks.DiffReviews).toHaveBeenCalledTimes(2)

    stop()
    fire()
    await flushPromises()
    expect(mocks.DiffReviews).toHaveBeenCalledTimes(2)
  })

  it('drops a dismissed comment at once, and its review with its last comment', async () => {
    mocks.DiffReviews.mockResolvedValue([review('r1', ['c1', 'c2'])])
    const store = useAgentReviews()
    store.watch('s1')
    await flushPromises()

    expect(await store.dismiss('s1', 'c1')).toBeNull()
    expect(store.reviews('s1')[0].comments?.map((comment) => comment.id)).toEqual(['c2'])
    await store.dismiss('s1', 'c2')
    expect(store.reviews('s1')).toEqual([])
  })

  it('reads the session again when a dismissal fails, which puts the comment back', async () => {
    mocks.DiffReviews.mockResolvedValue([review('r1', ['c1'])])
    mocks.DismissDiffComment.mockRejectedValue(new Error('gone wrong'))
    const store = useAgentReviews()
    store.watch('s1')
    await flushPromises()

    expect(await store.dismiss('s1', 'c1')).toBe('gone wrong')
    await flushPromises()
    expect(store.reviews('s1')).toHaveLength(1)
  })
})
