import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import IconX from '~icons/lucide/x'
import IconButton from '../IconButton.vue'
import Spinner from '../Spinner.vue'

function mountButton(props: Record<string, unknown> = {}) {
  return mount(IconButton, {
    props: { label: 'Close', icon: IconX, ...props },
    attrs: { 'data-testid': 'close' },
    attachTo: document.body,
  })
}

function bubble() {
  return document.body.querySelector('[data-testid="app-tooltip"]')
}

async function hover(wrapper: ReturnType<typeof mountButton>) {
  await wrapper.trigger('pointerenter')
  vi.advanceTimersByTime(300)
  await wrapper.vm.$nextTick()
}

describe('IconButton', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('names itself with the label and passes test ids and clicks through', async () => {
    const wrapper = mountButton()
    expect(wrapper.attributes('aria-label')).toBe('Close')
    expect(wrapper.attributes('data-testid')).toBe('close')
    expect(wrapper.attributes('title')).toBeUndefined()

    await wrapper.trigger('click')
    expect(wrapper.emitted('click')).toHaveLength(1)
  })

  it('shows the label as its tooltip after the dwell', async () => {
    const wrapper = mountButton()
    await hover(wrapper)
    expect(bubble()?.textContent).toBe('Close')
  })

  it('shows a longer tooltip without changing the accessible name', async () => {
    const wrapper = mountButton({ tooltip: 'Close -- the shell keeps running' })
    await hover(wrapper)
    expect(bubble()?.textContent).toBe('Close -- the shell keeps running')
    expect(wrapper.attributes('aria-label')).toBe('Close')
  })

  // The click usually opens a menu right where the bubble sits.
  it('drops the tooltip on press', async () => {
    const wrapper = mountButton()
    await hover(wrapper)
    await wrapper.trigger('pointerdown')
    expect(bubble()).toBeNull()
  })

  it('disables and spins while busy', () => {
    const wrapper = mountButton({ busy: true })
    expect(wrapper.attributes('disabled')).toBeDefined()
    expect(wrapper.attributes('aria-busy')).toBe('true')
    expect(wrapper.findComponent(Spinner).exists()).toBe(true)
  })

  it('disables without spinning', () => {
    const wrapper = mountButton({ disabled: true })
    expect(wrapper.attributes('disabled')).toBeDefined()
    expect(wrapper.attributes('aria-busy')).toBeUndefined()
    expect(wrapper.findComponent(Spinner).exists()).toBe(false)
  })
})
