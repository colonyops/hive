import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfigErrorOverlay from '../ConfigErrorOverlay.vue'
import type { ConfigValidationError } from '../../lib/configErrors'

function mountOverlay(errors: ConfigValidationError[], path = '/home/u/.config/hive/desktop/profiles.yaml') {
  return mount(ConfigErrorOverlay, { props: { path, errors } })
}

describe('ConfigErrorOverlay', () => {
  it('renders the config path and a single unstructured error entry', () => {
    const wrapper = mountOverlay([{ line: null, message: 'source "x": kind "search" requires a query' }])

    expect(document.querySelector('[data-testid="config-error-overlay"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="config-error-path"]')?.textContent).toContain('profiles.yaml')
    expect(document.querySelector('[data-testid="config-error-eyebrow"]')?.textContent).toContain('1 PROBLEM')
    expect(document.querySelector('[data-testid="config-error-subtitle"]')?.textContent).toContain('1 problem')

    const entries = document.querySelectorAll('[data-testid="config-error-entry"]')
    expect(entries).toHaveLength(1)
    expect(entries[0].textContent).toContain('kind "search" requires a query')
    // No line number was parsed, so no "line N" prefix renders.
    expect(entries[0].textContent).not.toContain('line')

    wrapper.unmount()
  })

  it('renders multiple line-numbered errors and pluralizes the counts', () => {
    const errors: ConfigValidationError[] = [
      { line: 4, message: 'field foo not found in type feed.configFile' },
      { line: 9, message: 'field bar not found in type feed.configFile' },
    ]
    const wrapper = mountOverlay(errors)

    expect(document.querySelector('[data-testid="config-error-eyebrow"]')?.textContent).toContain('2 PROBLEMS')
    expect(document.querySelector('[data-testid="config-error-subtitle"]')?.textContent).toContain('2 problems')

    const entries = document.querySelectorAll('[data-testid="config-error-entry"]')
    expect(entries).toHaveLength(2)
    expect(entries[0].textContent).toContain('line 4')
    expect(entries[0].textContent).toContain('field foo not found')
    expect(entries[1].textContent).toContain('line 9')

    wrapper.unmount()
  })

  it('emits retry, dismiss, copy-path, and copy-errors', async () => {
    const wrapper = mountOverlay([{ line: null, message: 'boom' }])

    await document.querySelector<HTMLButtonElement>('[data-testid="config-error-reload"]')?.click()
    await document.querySelector<HTMLButtonElement>('[data-testid="config-error-copy-path"]')?.click()
    await document.querySelector<HTMLButtonElement>('[data-testid="config-error-copy"]')?.click()
    await document.querySelector<HTMLButtonElement>('[data-testid="config-error-dismiss"]')?.click()

    expect(wrapper.emitted('retry')).toHaveLength(1)
    expect(wrapper.emitted('copy-path')).toHaveLength(1)
    expect(wrapper.emitted('copy-errors')).toHaveLength(1)
    expect(wrapper.emitted('dismiss')).toHaveLength(1)

    wrapper.unmount()
  })

  it('dismisses on Escape but not on a scrim click (hard block, no click-outside)', () => {
    const wrapper = mountOverlay([{ line: null, message: 'boom' }])

    document.querySelector<HTMLElement>('[data-testid="config-error-overlay"]')?.click()
    expect(wrapper.emitted('dismiss')).toBeUndefined()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('dismiss')).toHaveLength(1)

    wrapper.unmount()
  })

  // Regression: this overlay can appear over another sheet (e.g. the feed
  // editor) that also has its own unconditional, uncoordinated window Escape
  // listener holding unsaved form state. Escape must dismiss only the
  // topmost overlay, not also fire the sheet's own listener underneath.
  it('handles Escape in the capture phase, stopping it from reaching bubble-phase window listeners underneath', () => {
    const wrapper = mountOverlay([{ line: null, message: 'boom' }])

    // Stand-in for another sheet's own `window.addEventListener('keydown', ...)`
    // (no capture option — bubble phase, same as FeedEditorSheet/ConfigSheet/etc).
    const bubbleListener = vi.fn()
    window.addEventListener('keydown', bubbleListener)

    // Dispatched on document (not window) with bubbles:true so the event
    // actually travels capture-phase-down-then-bubble-phase-up through
    // window, like a real keydown originating on a focused element does —
    // dispatching directly on window collapses capture/bubble into a single
    // "at target" step where stopPropagation() has no effect to test.
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))

    expect(wrapper.emitted('dismiss')).toHaveLength(1)
    expect(bubbleListener).not.toHaveBeenCalled()

    window.removeEventListener('keydown', bubbleListener)
    wrapper.unmount()
  })
})
