import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import DriveReader from '../DriveReader.vue'
import { doc, documentFor } from '../../../test-utils/drive'

vi.mock('../../../composables/useWailsEvent', () => ({ useWailsEvent: vi.fn() }))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn().mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' }),
  ReviewLabels: vi.fn().mockResolvedValue(null),
}))
const reviews = vi.hoisted(() => ({
  Review: vi.fn().mockResolvedValue({ sessionId: '', current: true, comments: [] }),
  AddComment: vi.fn().mockResolvedValue({}),
}))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice',
  () => reviews,
)

describe('DriveReader comments', () => {
  it('opens the comment popover for a selection and saves it on the source lines', async () => {
    const wrapper = mount(DriveReader, {
      props: {
        document: documentFor('e2e/repo', doc('plans/p.md'), {
          markdown: '---\ntitle: P\n---\n\n# P\n\nThis plan is here.\n',
        }),
        paths: new Set(['plans/p.md']),
        loading: false,
        error: null,
        testid: 'r',
      },
      attachTo: document.body,
    })
    await flushPromises()

    const paragraph = wrapper.get('[data-testid="r-markdown"] p').element
    expect(paragraph.getAttribute('data-source-start')).toBe('7')
    const range = document.createRange()
    range.setStart(paragraph.firstChild!, 0)
    range.setEnd(paragraph.firstChild!, 9)
    const selection = window.getSelection()!
    selection.removeAllRanges()
    selection.addRange(range)
    document.dispatchEvent(new Event('selectionchange'))
    await flushPromises()

    await new Promise((resolve) => setTimeout(resolve, 250))
    await flushPromises()

    expect(document.querySelector('[data-testid="r-comment-popover"]')!.textContent).toContain('“This plan”')
    const text = document.querySelector<HTMLTextAreaElement>('[data-testid="r-comment-text"]')!
    text.value = 'Name it'
    text.dispatchEvent(new Event('input'))
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="r-comment-save"]')!.click()
    await flushPromises()
    expect(reviews.AddComment).toHaveBeenCalledWith('e2e/repo', 'plans/p.md', {
      startLine: 7,
      endLine: 7,
      context: 'This plan',
      text: 'Name it',
    })
  })

  it('comments on a whole code block from the handle beside it', async () => {
    reviews.AddComment.mockClear()
    const wrapper = mount(DriveReader, {
      props: {
        document: documentFor('e2e/repo', doc('plans/p.md'), { markdown: '# P\n\n```\na\nb\n```\n' }),
        paths: new Set(['plans/p.md']),
        loading: false,
        error: null,
        testid: 'r',
      },
      attachTo: document.body,
    })
    await flushPromises()

    wrapper.get('[data-testid="r-markdown"] code').element.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }))
    await flushPromises()
    await wrapper.get('[data-testid="r-block-comment"]').trigger('click')
    expect(window.getSelection()!.toString()).toBe('a\nb')
    document.dispatchEvent(new Event('selectionchange'))
    await new Promise((resolve) => setTimeout(resolve, 250))
    await flushPromises()

    const text = document.querySelector<HTMLTextAreaElement>('[data-testid="r-comment-text"]')!
    text.value = 'Split this'
    text.dispatchEvent(new Event('input'))
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="r-comment-save"]')!.click()
    await flushPromises()
    expect(reviews.AddComment).toHaveBeenCalledWith('e2e/repo', 'plans/p.md', {
      startLine: 4,
      endLine: 5,
      context: 'a\nb',
      text: 'Split this',
    })
    wrapper.unmount()
  })
})
