import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

const mocks = vi.hoisted(() => ({ SetText: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: mocks.SetText } }))

import { renderGithubMarkdown } from '../../lib/githubMarkdown'
import { useCodeCopy } from '../useCodeCopy'
import { resetToastsForTests, useToasts } from '../useToasts'

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
  mocks.SetText.mockReset().mockResolvedValue(undefined)
  resetToastsForTests()
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

    expect(mocks.SetText).toHaveBeenCalledTimes(1)
    expect(mocks.SetText).toHaveBeenCalledWith('b := 2\nc := 3')
    expect(wrapper.emitted('handled')).toEqual([[true]])
    expect(button.classes()).toContain('is-copied')

    vi.advanceTimersByTime(1500)
    expect(button.classes()).not.toContain('is-copied')
  })

  it('leaves a click outside a copy button to the host', async () => {
    const wrapper = mount(Host, { props: { source } })

    await wrapper.get('p').trigger('click')
    await flushPromises()

    expect(mocks.SetText).not.toHaveBeenCalled()
    expect(wrapper.emitted('handled')).toEqual([[false]])
  })

  it('reports a failed copy with a toast and no check', async () => {
    mocks.SetText.mockRejectedValue(new Error('no clipboard'))
    const wrapper = mount(Host, { props: { source } })
    const button = wrapper.get('button.markdown-code-copy')

    await button.trigger('click')
    await flushPromises()

    const { toasts } = useToasts()
    expect(toasts.value.map((toast) => [toast.message, toast.severity])).toEqual([
      ['Could not copy to the clipboard', 'error'],
    ])
    expect(button.classes()).not.toContain('is-copied')
  })
})
