import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DriveCommentPopover from '../DriveCommentPopover.vue'

const settings = vi.hoisted(() => ({ ReviewLabels: vi.fn().mockResolvedValue(null) }))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settings,
)

function mountPopover() {
  return mount(DriveCommentPopover, {
    props: {
      rect: { top: 100, left: 50, width: 900, height: 60 },
      range: { startLine: 8, endLine: 9, context: 'This plan' },
      busy: false,
      error: null,
      testid: 'r',
    },
    attachTo: document.body,
  })
}

afterEach(() => settings.ReviewLabels.mockResolvedValue(null))

const press = (key: string) => document.body.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))

describe('DriveCommentPopover', () => {
  it('starts the comment from a typed letter while nothing has focus', async () => {
    mountPopover()
    await flushPromises()
    press('W')
    press('h')
    await flushPromises()
    const field = document.querySelector<HTMLTextAreaElement>('[data-testid="r-comment-text"]')!
    expect(field.value).toBe('Wh')
    expect(document.activeElement).toBe(field)
  })

  it('applies a label from its key', async () => {
    const wrapper = mountPopover()
    await flushPromises()
    press('3')
    expect(wrapper.emitted('save')).toEqual([['Needs tests']])
  })

  it('offers the labels settings.yaml configures', async () => {
    settings.ReviewLabels.mockResolvedValue([{ title: 'Nit', icon: 'scissors' }])
    const wrapper = mountPopover()
    await flushPromises()
    expect(document.querySelector('[data-testid="r-comment-label-2"]')).toBeNull()
    press('1')
    expect(wrapper.emitted('save')).toEqual([['Nit']])
  })

  it('floats under the selection at its own width and drags by its top row', async () => {
    mount(DriveCommentPopover, {
      props: {
        rect: { top: 100, left: 50, width: 900, height: 60 },
        range: { startLine: 8, endLine: 9, context: 'This plan' },
        busy: false,
        error: null,
        testid: 'r',
      },
      attachTo: document.body,
    })
    await flushPromises()

    const popover = document.querySelector<HTMLElement>('[data-testid="r-comment-popover"]')!
    expect(popover.classList).toContain('w-[360px]')
    expect(popover.style.top).toBe('168px')
    expect(popover.style.left).toBe('50px')
    expect(popover.textContent).toContain('L8–9')

    const grip = document.querySelector<HTMLElement>('[data-testid="r-comment-grip"]')!
    grip.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0, clientX: 10, clientY: 10 }))
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 40, clientY: 50 }))
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 40, clientY: 50 }))
    await flushPromises()

    expect(popover.style.top).toBe('208px')
    expect(popover.style.left).toBe('80px')
  })
})
