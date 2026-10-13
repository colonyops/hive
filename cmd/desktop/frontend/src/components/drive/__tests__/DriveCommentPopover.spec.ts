import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import DriveCommentPopover from '../DriveCommentPopover.vue'

describe('DriveCommentPopover', () => {
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
