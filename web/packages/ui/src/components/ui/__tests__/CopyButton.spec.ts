import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { fakePlatform } from '../../../test-utils/platform'
import CopyButton from '../CopyButton.vue'

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('CopyButton', () => {
  it('copies its text and shows Copied until the reset', async () => {
    const wrapper = mount(CopyButton, { props: { text: 'abc123', label: 'Copy id' } })

    await wrapper.trigger('click')
    await flushPromises()
    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('abc123')
    expect(wrapper.text()).toBe('Copied')

    vi.advanceTimersByTime(2000)
    await flushPromises()
    expect(wrapper.text()).toBe('Copy id')
  })

  it('reports whether the copy reached the clipboard', async () => {
    fakePlatform.clipboard.writeText.mockRejectedValueOnce(new Error('denied'))
    const wrapper = mount(CopyButton, { props: { text: 'abc' } })

    await wrapper.trigger('click')
    await flushPromises()
    await wrapper.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('copy')).toEqual([[false], [true]])
  })

  it('does nothing with empty text', async () => {
    const wrapper = mount(CopyButton, { props: { text: '' } })

    await wrapper.trigger('click')
    await flushPromises()

    expect(fakePlatform.clipboard.writeText).not.toHaveBeenCalled()
    expect(wrapper.text()).toBe('Copy')
  })

  it('passes a test id through to the button', () => {
    const wrapper = mount(CopyButton, { props: { text: 'x' }, attrs: { 'data-testid': 'copy' } })

    expect(wrapper.attributes('data-testid')).toBe('copy')
  })
})
