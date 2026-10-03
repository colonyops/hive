import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({ SetText: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: mocks.SetText } }))

import CopyButton from '../CopyButton.vue'

beforeEach(() => {
  vi.useFakeTimers()
  mocks.SetText.mockReset().mockResolvedValue(undefined)
})

afterEach(() => {
  vi.useRealTimers()
})

describe('CopyButton', () => {
  it.each(['button', 'icon', 'link'] as const)('copies its text as a %s', async (variant) => {
    const wrapper = mount(CopyButton, {
      props: { text: 'abc123', label: 'Copy id', variant },
      attrs: { 'data-testid': 'copy' },
    })

    await wrapper.get('[data-testid="copy"]').trigger('click')
    await flushPromises()

    expect(mocks.SetText).toHaveBeenCalledWith('abc123')
    wrapper.unmount()
  })

  it('shows Copied until the reset delay passes', async () => {
    const wrapper = mount(CopyButton, {
      props: { text: 'abc123', label: 'Copy id', variant: 'link', resetDelay: 500 },
    })

    await wrapper.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toBe('Copied')

    vi.advanceTimersByTime(500)
    await flushPromises()
    expect(wrapper.text()).toBe('Copy id')
  })

  it('does nothing with empty text', async () => {
    const wrapper = mount(CopyButton, { props: { text: '', label: 'Copy id', variant: 'link' } })

    await wrapper.trigger('click')
    await flushPromises()

    expect(mocks.SetText).not.toHaveBeenCalled()
    expect(wrapper.text()).toBe('Copy id')
  })

  it('names an icon button by its label', () => {
    const wrapper = mount(CopyButton, {
      props: { text: 'x', label: 'Copy link', variant: 'icon' },
      attrs: { 'data-testid': 'copy' },
    })

    expect(wrapper.get('[data-testid="copy"]').attributes('aria-label')).toBe('Copy link')
    wrapper.unmount()
  })
})
