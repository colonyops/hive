import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DriveReviewPanel from '../DriveReviewPanel.vue'
import type { DocumentReviewWire } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

vi.mock('../../../composables/useWailsEvent', () => ({ useWailsEvent: vi.fn() }))

const runtime = vi.hoisted(() => ({ setText: vi.fn().mockResolvedValue(undefined) }))
vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: runtime.setText } }))

const reviews = vi.hoisted(() => ({
  Review: vi.fn(),
  AddComment: vi.fn(),
  UpdateComment: vi.fn().mockResolvedValue(undefined),
  DeleteComment: vi.fn().mockResolvedValue(undefined),
  Feedback: vi.fn().mockResolvedValue('Document: /abs/plans/p.md\n'),
  Clear: vi.fn().mockResolvedValue(undefined),
  Send: vi.fn(),
  AgentTargets: vi.fn().mockResolvedValue([]),
}))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice',
  () => reviews,
)

function reviewOf(overrides: Partial<DocumentReviewWire> = {}): DocumentReviewWire {
  return {
    sessionId: 's',
    current: true,
    comments: [
      { id: 'c1', startLine: 6, endLine: 6, context: 'Step one.', text: 'Tighten this', createdAt: '' },
      { id: 'c2', startLine: 8, endLine: 10, context: 'Long\nquote', text: 'Why?', createdAt: '' },
    ],
    ...overrides,
  }
}

async function mountPanel() {
  const wrapper = mount(DriveReviewPanel, {
    props: { space: 'workspaces/ws', path: 'plans/p.md', testid: 'r' },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

describe('DriveReviewPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    reviews.Review.mockResolvedValue(reviewOf())
  })

  it('renders nothing without comments or a note', async () => {
    reviews.Review.mockResolvedValue({ sessionId: '', current: true, comments: [] })
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-testid="r-review"]').exists()).toBe(false)
  })

  it('lists each comment with its lines, the first line of its quote, and its text', async () => {
    const wrapper = await mountPanel()
    const second = wrapper.get('[data-testid="r-review-row-2"]')
    expect(second.text()).toContain('L8-10')
    expect(second.text()).toContain('Long')
    expect(second.text()).not.toContain('quote')
    expect(second.text()).toContain('Why?')
    expect(wrapper.get('[data-testid="r-review-row-1"]').text()).toContain('L6')
  })

  it('edits and deletes a comment', async () => {
    const wrapper = await mountPanel()
    await wrapper.get('[data-testid="r-review-edit-1"]').trigger('click')
    await wrapper.get('[data-testid="r-review-edit-text-1"]').setValue('Tighter')
    await wrapper.get('[data-testid="r-review-edit-save-1"]').trigger('click')
    await flushPromises()
    expect(reviews.UpdateComment).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md', 'c1', 'Tighter')

    await wrapper.get('[data-testid="r-review-delete-2"]').trigger('click')
    await flushPromises()
    expect(reviews.DeleteComment).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md', 'c2')
  })

  it('copies the feedback with the note', async () => {
    const wrapper = await mountPanel()
    await wrapper.get('[data-testid="r-review-note"]').setValue('Overall fine')
    await wrapper.get('[data-testid="r-review-copy"]').trigger('click')
    await flushPromises()
    expect(reviews.Feedback).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md', 'Overall fine')
    expect(runtime.setText).toHaveBeenCalledWith('Document: /abs/plans/p.md\n')
    expect(reviews.Clear).not.toHaveBeenCalled()
  })

  it('asks before clearing', async () => {
    const wrapper = await mountPanel()
    await wrapper.get('[data-testid="r-review-clear"]').trigger('click')
    expect(reviews.Clear).not.toHaveBeenCalled()
    reviews.Review.mockResolvedValue({ sessionId: '', current: true, comments: [] })
    await wrapper.get('[data-testid="r-review-confirm-confirm"]').trigger('click')
    await flushPromises()
    expect(reviews.Clear).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md')
    expect(wrapper.find('[data-testid="r-review"]').exists()).toBe(false)
  })

  it('shows comments on an earlier version with a way to discard them', async () => {
    reviews.Review.mockResolvedValue(reviewOf({ current: false }))
    const wrapper = await mountPanel()
    expect(wrapper.get('[data-testid="r-review-stale"]').text()).toContain('earlier version')
    await wrapper.get('[data-testid="r-review-discard"]').trigger('click')
    await wrapper.get('[data-testid="r-review-confirm-confirm"]').trigger('click')
    await flushPromises()
    expect(reviews.Clear).toHaveBeenCalled()
  })
})
