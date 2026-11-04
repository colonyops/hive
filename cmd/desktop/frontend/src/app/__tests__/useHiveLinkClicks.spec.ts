import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { useHiveLinkClicks } from '../useHiveLinkClicks'

const mocks = vi.hoisted(() => ({ Open: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/linkservice', () => mocks)

describe('useHiveLinkClicks', () => {
  const showError = vi.fn()
  const Host = defineComponent({
    setup() {
      useHiveLinkClicks(showError)
      return () =>
        h('div', [
          h('a', { href: 'hive://drive/colonyops/hive/plans/x.md', 'data-testid': 'hive' }, h('span', 'plan')),
          h('a', { href: 'https://example.com', 'data-testid': 'web' }, 'web'),
        ])
    },
  })

  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Open.mockResolvedValue(undefined)
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('opens a hive:// link in the app, from a click on anything inside it', async () => {
    const wrapper = mount(Host, { attachTo: document.body })
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    wrapper.get('[data-testid="hive"] span').element.dispatchEvent(event)
    await flushPromises()
    expect(mocks.Open).toHaveBeenCalledWith('hive://drive/colonyops/hive/plans/x.md')
    expect(event.defaultPrevented).toBe(true)
    wrapper.unmount()
  })

  it('leaves other links to the surface they are on', async () => {
    const wrapper = mount(Host, { attachTo: document.body })
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    wrapper.get('[data-testid="web"]').element.dispatchEvent(event)
    await flushPromises()
    expect(mocks.Open).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
    wrapper.unmount()
  })

  it('says why when Go refuses the link', async () => {
    mocks.Open.mockRejectedValue(new Error('"hive://nowhere" is not a hive:// link Hive can open'))
    const wrapper = mount(Host, { attachTo: document.body })
    await wrapper.get('[data-testid="hive"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('not a hive:// link'))
    wrapper.unmount()
  })
})
