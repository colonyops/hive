import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDriveReview } from '../useDriveReview'

const api = vi.hoisted(() => ({ Review: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice', () => api)

const wailsEvents = vi.hoisted(() => ({ handlers: new Map<string, (event: { data: unknown }) => void>() }))
vi.mock('../../composables/useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: (event: { data: unknown }) => void) => wailsEvents.handlers.set(name, handler),
}))

const reviewUpdated = (data: unknown) => wailsEvents.handlers.get('review:updated')?.({ data })

describe('useDriveReview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.Review.mockResolvedValue({ sessionId: '', current: true, comments: [] })
  })

  it('re-reads a held review when another window changes its comments', async () => {
    const review = useDriveReview().review('ws', 'a.md')
    await flushPromises()

    api.Review.mockResolvedValue({ sessionId: 's', current: true, comments: [{ id: 'c1' }] })
    reviewUpdated({ space: 'ws', path: 'a.md' })
    await flushPromises()

    expect(review.data.value.comments).toEqual([{ id: 'c1' }])
  })

  it('leaves other documents alone and does not load one nobody asked for', async () => {
    useDriveReview().review('ws', 'a.md')
    await flushPromises()
    api.Review.mockClear()

    reviewUpdated({ space: 'ws', path: 'b.md' })
    await flushPromises()

    expect(api.Review).not.toHaveBeenCalled()
  })
})
