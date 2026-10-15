import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { useAutofocus } from '../useAutofocus'

describe('useAutofocus', () => {
  it('focuses its target after mounting', async () => {
    const FocusTarget = defineComponent({
      setup() {
        const target = ref<HTMLInputElement | null>(null)
        useAutofocus(target)
        return () => h('input', { ref: target, 'data-testid': 'focus-target' })
      },
    })
    const wrapper = mount(FocusTarget, { attachTo: document.body })

    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('[data-testid="focus-target"]').element)

    wrapper.unmount()
  })
})
