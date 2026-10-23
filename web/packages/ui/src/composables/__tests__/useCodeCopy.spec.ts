import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

import { renderGithubMarkdown } from '../../lib/githubMarkdown'
import { useCodeCopy } from '../useCodeCopy'
import { fakePlatform } from '../../test-utils/platform'

// A host the way the panes use the composable: rendered markdown through
// innerHTML, one delegated click handler, and the handler's answer emitted so
// a test can see whether the click was a copy.
const Host = defineComponent({
  props: { source: { type: String, required: true } },
  emits: ['handled'],
  setup(props, { emit }) {
    const { copyCode } = useCodeCopy()
    return () =>
      h('div', {
        innerHTML: renderGithubMarkdown(props.source),
        onClick: (event: MouseEvent) => emit('handled', copyCode(event)),
      })
  },
})

const source = ['```go', 'a := 1', '```', '', 'between', '', '```go', 'b := 2', 'c := 3', '```'].join('\n')

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useCodeCopy', () => {
  it('copies the clicked block without its trailing newline and shows the check until the reset', async () => {
    const wrapper = mount(Host, { props: { source } })
    const button = wrapper.findAll('button.markdown-code-copy')[1]

    await button.trigger('click')
    await flushPromises()

    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledTimes(1)
    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('b := 2\nc := 3')
    expect(wrapper.emitted('handled')).toEqual([[true]])
    expect(button.classes()).toContain('is-copied')

    vi.advanceTimersByTime(1500)
    expect(button.classes()).not.toContain('is-copied')
  })

  it('leaves a click outside a copy button to the host', async () => {
    const wrapper = mount(Host, { props: { source } })

    await wrapper.get('p').trigger('click')
    await flushPromises()

    expect(fakePlatform.clipboard.writeText).not.toHaveBeenCalled()
    expect(wrapper.emitted('handled')).toEqual([[false]])
  })

  it('reports a failed copy with a toast and no check', async () => {
    fakePlatform.clipboard.writeText.mockRejectedValue(new Error('no clipboard'))
    const wrapper = mount(Host, { props: { source } })
    const button = wrapper.get('button.markdown-code-copy')

    await button.trigger('click')
    await flushPromises()

    expect(fakePlatform.toast).toHaveBeenCalledExactlyOnceWith('Could not copy to the clipboard')
    expect(button.classes()).not.toContain('is-copied')
  })
})
