import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppTooltip from '../AppTooltip.vue'

function mountTooltip(props: Record<string, unknown> = {}) {
  return mount(AppTooltip, {
    props: { text: 'Uncommitted changes', ...props },
    slots: { default: '<button data-testid="trigger">x</button>' },
    attachTo: document.body,
  })
}

function bubble() {
  return document.body.querySelector('[data-testid="app-tooltip"]')
}

describe('AppTooltip', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  // The reason this component exists: `title` waits on the platform, which is
  // over a second in WebKit and has no knob.
  it('appears after a short dwell rather than immediately or on the platform delay', async () => {
    const wrapper = mountTooltip()
    await wrapper.trigger('pointerenter')
    expect(bubble()).toBeNull()

    vi.advanceTimersByTime(299)
    await wrapper.vm.$nextTick()
    expect(bubble()).toBeNull()

    vi.advanceTimersByTime(1)
    await wrapper.vm.$nextTick()
    expect(bubble()?.textContent).toBe('Uncommitted changes')
  })

  it('takes a caller-chosen dwell', async () => {
    const wrapper = mountTooltip({ delay: 400 })
    await wrapper.trigger('pointerenter')
    vi.advanceTimersByTime(200)
    await wrapper.vm.$nextTick()
    expect(bubble()).toBeNull()

    vi.advanceTimersByTime(200)
    await wrapper.vm.$nextTick()
    expect(bubble()).not.toBeNull()
  })

  // Cursor transit across a dense row of chips must not leave a trail of
  // tooltips behind it.
  it('never appears when the pointer leaves inside the dwell', async () => {
    const wrapper = mountTooltip()
    await wrapper.trigger('pointerenter')
    vi.advanceTimersByTime(100)
    await wrapper.trigger('pointerleave')
    vi.advanceTimersByTime(500)
    await wrapper.vm.$nextTick()
    expect(bubble()).toBeNull()
  })

  // Arriving by Tab is already deliberate; there is no cursor passing through.
  it('skips the dwell for keyboard focus', async () => {
    const wrapper = mountTooltip()
    await wrapper.trigger('focusin')
    await wrapper.vm.$nextTick()
    expect(bubble()).not.toBeNull()
  })

  it('shows nothing at all without text', async () => {
    const wrapper = mountTooltip({ text: '' })
    await wrapper.trigger('pointerenter')
    vi.advanceTimersByTime(500)
    await wrapper.vm.$nextTick()
    expect(bubble()).toBeNull()
    expect(wrapper.find('[data-testid="trigger"]').exists()).toBe(true)
  })

  it('takes the bubble down with the component', async () => {
    const wrapper = mountTooltip()
    await wrapper.trigger('focusin')
    await wrapper.vm.$nextTick()
    expect(bubble()).not.toBeNull()

    wrapper.unmount()
    await wrapper.vm.$nextTick()
    expect(bubble()).toBeNull()
  })
})
