import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Component } from 'vue'
import DriveReviewCards from '../DriveReviewCards.vue'
import DriveReviewFooter from '../DriveReviewFooter.vue'
import DriveReviewHeader from '../DriveReviewHeader.vue'
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

const props = { space: 'workspaces/ws', path: 'plans/p.md', testid: 'r' }

async function mountPart(component: Component) {
  const wrapper = mount(component, { props, attachTo: document.body })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  reviews.Review.mockResolvedValue(reviewOf())
})

describe('DriveReviewHeader', () => {
  it('counts the comments and asks before clearing', async () => {
    const wrapper = await mountPart(DriveReviewHeader)
    expect(wrapper.get('[data-testid="r-review-count"]').text()).toBe('2')

    await wrapper.get('[data-testid="r-review-clear"]').trigger('click')
    expect(reviews.Clear).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="r-review-clear-confirm"]').trigger('click')
    await flushPromises()
    expect(reviews.Clear).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md')
  })
})

describe('DriveReviewCards', () => {
  it('shows each comment with its lines and text, without the quote', async () => {
    const wrapper = await mountPart(DriveReviewCards)
    const second = wrapper.get('[data-testid="r-review-card-2"]')
    expect(second.text()).toContain('L8–10')
    expect(second.text()).toContain('Why?')
    expect(second.text()).not.toContain('quote')
  })

  it('edits and deletes from every card in one click', async () => {
    const wrapper = await mountPart(DriveReviewCards)
    expect(wrapper.find('[data-testid="r-review-delete-2"]').exists()).toBe(true)

    await wrapper.get('[data-testid="r-review-edit-1"]').trigger('click')
    await wrapper.get('[data-testid="r-review-edit-text-1"]').setValue('Tighter')
    await wrapper.get('[data-testid="r-review-edit-save-1"]').trigger('click')
    await flushPromises()
    expect(reviews.UpdateComment).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md', 'c1', 'Tighter')

    await wrapper.get('[data-testid="r-review-delete-1"]').trigger('click')
    await flushPromises()
    expect(reviews.DeleteComment).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md', 'c1')
  })

  it('marks the card that was picked', async () => {
    const wrapper = await mountPart(DriveReviewCards)
    await wrapper.get('[data-testid="r-review-card-1"]').trigger('click')
    expect(wrapper.get('[data-testid="r-review-card-1"]').classes()).toContain('border-accent/45')
  })

  it('lights the card under the pointer', async () => {
    const wrapper = await mountPart(DriveReviewCards)
    await wrapper.get('[data-testid="r-review-card-2"]').trigger('mouseenter')
    expect(wrapper.get('[data-testid="r-review-card-2"]').classes()).toContain('border-strong')
    await wrapper.get('[data-testid="r-review-card-2"]').trigger('mouseleave')
    expect(wrapper.get('[data-testid="r-review-card-2"]').classes()).not.toContain('border-strong')
  })

  it('places the cards of a newly opened document without sliding them', async () => {
    reviews.Review.mockImplementation((_space: string, path: string) =>
      Promise.resolve(
        path === 'plans/p.md'
          ? reviewOf()
          : reviewOf({ comments: [{ id: 'c3', startLine: 1, endLine: 1, context: '', text: 'New', createdAt: '' }] }),
      ),
    )
    const reader = document.createElement('div')
    reader.innerHTML = '<div data-testid="r-document"><p data-review-ids="c1"></p><p data-review-ids="c2"></p></div>'
    document.body.append(reader)
    const wrapper = mount(DriveReviewCards, { props: { ...props, alignTo: reader }, attachTo: document.body })
    await flushPromises()
    await wrapper.get('[data-testid="r-review-card-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="r-review-card-1"]').classes()).toContain(
      'transition-[top,border-color,background-color]',
    )

    await wrapper.setProps({ path: 'plans/q.md' })
    await flushPromises()
    reader.innerHTML = '<div data-testid="r-document"><p data-review-ids="c3"></p></div>'
    await flushPromises()
    const card = wrapper.get('[data-testid="r-review-card-1"]')
    expect(card.text()).toContain('New')
    expect(card.classes()).not.toContain('transition-[top,border-color,background-color]')
    reader.remove()
  })

  it('shows comments on an earlier version with a way to discard them', async () => {
    reviews.Review.mockResolvedValue(reviewOf({ current: false }))
    const wrapper = await mountPart(DriveReviewCards)
    expect(wrapper.get('[data-testid="r-review-stale"]').text()).toContain('earlier version')
    await wrapper.get('[data-testid="r-review-discard"]').trigger('click')
    await wrapper.get('[data-testid="r-review-confirm-confirm"]').trigger('click')
    await flushPromises()
    expect(reviews.Clear).toHaveBeenCalled()
  })
})

describe('DriveReviewFooter', () => {
  it('counts what is ready, copies it all, and opens the send dialog on ⌘↵ in the note', async () => {
    const wrapper = await mountPart(DriveReviewFooter)
    expect(wrapper.get('[data-testid="r-review-ready"]').text()).toBe('2 comments ready')

    await wrapper.get('[data-testid="r-review-copy"]').trigger('click')
    await flushPromises()
    expect(reviews.Feedback).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md', '')
    expect(runtime.setText).toHaveBeenCalledWith('Document: /abs/plans/p.md\n')

    const note = wrapper.get('[data-testid="r-review-note"]')
    await note.setValue('Overall fine')
    await note.trigger('keydown', { key: 'Enter', metaKey: true })
    await flushPromises()
    expect(document.querySelector('[data-testid="r-send-dialog"]')).not.toBeNull()
    wrapper.unmount()
  })

  async function pickAgentAndSubmit(wrapper: Awaited<ReturnType<typeof mountPart>>, note: string) {
    reviews.AgentTargets.mockResolvedValue([{ kind: 'session', id: 's1', pane: '%1', label: 'app', status: 'ready' }])
    await wrapper.get('[data-testid="r-review-note"]').setValue(note)
    await wrapper.get('[data-testid="r-review-send"]').trigger('click')
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="r-send-target-0"]')!.click()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="r-send-submit"]')!.click()
    await flushPromises()
  }

  it('sends the document, the note, and the picked agent, then clears the note', async () => {
    reviews.Send.mockResolvedValue(undefined)
    const wrapper = await mountPart(DriveReviewFooter)
    await pickAgentAndSubmit(wrapper, 'Ship it')

    expect(reviews.Send).toHaveBeenCalledExactlyOnceWith('workspaces/ws', 'plans/p.md', 'Ship it', {
      kind: 'session',
      id: 's1',
      pane: '%1',
    })
    expect(document.querySelector('[data-testid="r-send-dialog"]')).toBeNull()
    expect(wrapper.get<HTMLTextAreaElement>('[data-testid="r-review-note"]').element.value).toBe('')
    wrapper.unmount()
  })

  it('shows a rejected send in the dialog and keeps the note', async () => {
    reviews.Send.mockRejectedValue(new Error('pane gone'))
    const wrapper = await mountPart(DriveReviewFooter)
    await pickAgentAndSubmit(wrapper, 'Keep me')

    expect(document.querySelector('[data-testid="r-send-error"]')!.textContent).toContain('pane gone')
    expect(wrapper.get<HTMLTextAreaElement>('[data-testid="r-review-note"]').element.value).toBe('Keep me')
    wrapper.unmount()
  })
})
