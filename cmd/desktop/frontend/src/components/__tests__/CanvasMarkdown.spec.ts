import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CanvasMarkdown from '../CanvasMarkdown.vue'

describe('CanvasMarkdown', () => {
  it('prevents empty links from navigating the webview', () => {
    const wrapper = mount(CanvasMarkdown, {
      props: { source: '[stay here]()', testid: 'canvas-markdown' },
    })
    const click = new MouseEvent('click', { bubbles: true, cancelable: true })

    wrapper.get('a').element.dispatchEvent(click)

    expect(click.defaultPrevented).toBe(true)
    expect(wrapper.emitted('follow')).toBeUndefined()
  })
})
