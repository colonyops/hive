import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import HtmlDocumentFrame from '../HtmlDocumentFrame.vue'

describe('HtmlDocumentFrame', () => {
  it.each([{}, { src: '/raw/page.html' }])('sandboxes to scripts alone with %j', (extra) => {
    const wrapper = mount(HtmlDocumentFrame, { props: { html: '<p>Hi</p>', testid: 'f', ...extra } })
    expect(wrapper.get('iframe').attributes('sandbox')).toBe('allow-scripts')
  })

  it('keeps its stacking inside its own context', () => {
    const wrapper = mount(HtmlDocumentFrame, { props: { html: '<p>Hi</p>', testid: 'f' } })
    expect(wrapper.classes()).toContain('isolate')
    expect(
      wrapper
        .get('iframe')
        .classes()
        .some((c) => c.startsWith('z-')),
    ).toBe(false)
  })
})
