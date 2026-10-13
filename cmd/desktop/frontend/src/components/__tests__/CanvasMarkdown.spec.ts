import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ SetText: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: mocks.SetText } }))

import CanvasMarkdown from '../CanvasMarkdown.vue'

beforeEach(() => {
  mocks.SetText.mockReset().mockResolvedValue(undefined)
})

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

  it('copies a code block on its button and does not treat the click as a link', async () => {
    const wrapper = mount(CanvasMarkdown, {
      props: { source: '```sh\necho hi\n```', testid: 'canvas-markdown' },
    })

    await wrapper.get('button.markdown-code-copy').trigger('click')
    await flushPromises()

    expect(mocks.SetText).toHaveBeenCalledWith('echo hi')
    expect(wrapper.emitted('follow')).toBeUndefined()
  })
})
