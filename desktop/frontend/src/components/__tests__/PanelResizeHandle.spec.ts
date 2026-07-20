import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PanelResizeHandle from '../PanelResizeHandle.vue'

function mountHandle(edge: 'left' | 'right' = 'right') {
  const start = vi.fn()
  const step = vi.fn()
  const wrapper = mount(PanelResizeHandle, { props: { edge, name: 'demo', start, step } })
  return { wrapper, start, step }
}

describe('PanelResizeHandle', () => {
  it('renders with its data-testid and separator semantics', () => {
    const { wrapper } = mountHandle()
    const el = wrapper.get('[data-testid="resize-handle-demo"]')
    expect(el.attributes('role')).toBe('separator')
    expect(el.attributes('aria-orientation')).toBe('vertical')
    expect(el.attributes('tabindex')).toBe('0')
  })

  it('invokes the start prop on pointerdown', async () => {
    const { wrapper, start } = mountHandle()
    await wrapper.trigger('pointerdown', { clientX: 100, pointerId: 1 })
    expect(start).toHaveBeenCalledTimes(1)
  })

  it('nudges via step() on Arrow Left/Right, in opposite directions', async () => {
    const { wrapper, step } = mountHandle()

    await wrapper.trigger('keydown', { key: 'ArrowRight' })
    expect(step).toHaveBeenLastCalledWith(12)

    await wrapper.trigger('keydown', { key: 'ArrowLeft' })
    expect(step).toHaveBeenLastCalledWith(-12)
  })

  it('ignores other keys', async () => {
    const { wrapper, step } = mountHandle()
    await wrapper.trigger('keydown', { key: 'Enter' })
    expect(step).not.toHaveBeenCalled()
  })
})
