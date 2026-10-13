import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import HtmlDocumentFrame from '../HtmlDocumentFrame.vue'
import { useOpenModalCount } from '../../../composables/useOpenModalCount'

function mountFrame() {
  return mount(HtmlDocumentFrame, {
    props: { html: '<html><body>demo</body></html>', testid: 'r' },
    attachTo: document.body,
  })
}

describe('HtmlDocumentFrame', () => {
  it('expands the same frame over its drive view, so a running demo keeps its state', async () => {
    const wrapper = mountFrame()
    const frame = wrapper.get('[data-testid="r-frame"]').element

    await wrapper.get('[data-testid="r-frame-expand"]').trigger('click')

    expect(wrapper.classes()).toContain('absolute')
    expect(wrapper.get('[data-testid="r-frame"]').element).toBe(frame)
    expect(useOpenModalCount().value).toBe(1)

    await wrapper.get('[data-testid="r-frame-expand"]').trigger('click')
    expect(wrapper.classes()).not.toContain('absolute')
    expect(useOpenModalCount().value).toBe(0)
    wrapper.unmount()
  })

  it('collapses on Escape', async () => {
    const wrapper = mountFrame()
    await wrapper.get('[data-testid="r-frame-expand"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()

    expect(wrapper.classes()).not.toContain('absolute')
    expect(useOpenModalCount().value).toBe(0)
    wrapper.unmount()
  })

  it('releases the modal count when unmounted while expanded', async () => {
    const wrapper = mountFrame()
    await wrapper.get('[data-testid="r-frame-expand"]').trigger('click')

    wrapper.unmount()

    expect(useOpenModalCount().value).toBe(0)
  })
})
