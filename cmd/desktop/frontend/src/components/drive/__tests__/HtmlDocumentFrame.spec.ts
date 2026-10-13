import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import HtmlDocumentFrame from '../HtmlDocumentFrame.vue'

describe('HtmlDocumentFrame', () => {
  it('fills the reader when expanded and keeps the same frame when collapsed, so a running demo keeps its state', async () => {
    const wrapper = mount(HtmlDocumentFrame, {
      props: { html: '<html><body>demo</body></html>', expanded: true, testid: 'r' },
    })
    const frame = wrapper.get('[data-testid="r-frame"]')
    expect(frame.classes()).toContain('absolute')

    await wrapper.setProps({ expanded: false })

    expect(wrapper.get('[data-testid="r-frame"]').element).toBe(frame.element)
    expect(frame.classes()).not.toContain('absolute')
  })
})
