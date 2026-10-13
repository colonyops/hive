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
  it('opens filling the reader without counting as a modal', () => {
    const wrapper = mountFrame()

    expect(wrapper.classes()).toContain('absolute')
    expect(useOpenModalCount().value).toBe(0)
    wrapper.unmount()
  })

  it('collapses and expands the same frame, so a running demo keeps its state', async () => {
    const wrapper = mountFrame()
    const frame = wrapper.get('[data-testid="r-frame"]').element

    await wrapper.get('[data-testid="r-frame-expand"]').trigger('click')
    expect(wrapper.classes()).not.toContain('absolute')
    expect(wrapper.get('[data-testid="r-frame"]').element).toBe(frame)

    await wrapper.get('[data-testid="r-frame-expand"]').trigger('click')
    expect(wrapper.classes()).toContain('absolute')
    expect(wrapper.get('[data-testid="r-frame"]').element).toBe(frame)
    wrapper.unmount()
  })
})
