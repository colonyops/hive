import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SearchField from '../SearchField.vue'

const onWindowKeydown = vi.fn()

afterEach(() => {
  window.removeEventListener('keydown', onWindowKeydown)
  onWindowKeydown.mockReset()
})

function mountField(props: Record<string, unknown> = {}, attrs: Record<string, unknown> = {}) {
  window.addEventListener('keydown', onWindowKeydown)
  return mount(SearchField, {
    props: { testid: 'thing-search', ariaLabel: 'Filter things', ...props },
    attrs,
    attachTo: document.body,
  })
}

function pressEscape(input: Element): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
  input.dispatchEvent(event)
  return event
}

describe('SearchField', () => {
  it('shows modelValue and emits each edit', async () => {
    const wrapper = mountField({ modelValue: 'old' })
    const input = wrapper.get('[data-testid="thing-search"]')

    expect((input.element as HTMLInputElement).value).toBe('old')
    expect(input.attributes('aria-label')).toBe('Filter things')
    await input.setValue('new')
    expect(wrapper.emitted('update:modelValue')).toEqual([['new']])
    wrapper.unmount()
  })

  it('clears on Escape when it has text, and keeps the key from the window', () => {
    const wrapper = mountField({ modelValue: 'query' })

    const event = pressEscape(wrapper.get('input').element)

    expect(wrapper.emitted('update:modelValue')).toEqual([['']])
    expect(wrapper.emitted('escape')).toBeUndefined()
    expect(event.defaultPrevented).toBe(true)
    expect(onWindowKeydown).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('lets Escape reach the window when empty, and emits escape', () => {
    const wrapper = mountField({ modelValue: '' })

    pressEscape(wrapper.get('input').element)

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('escape')).toHaveLength(1)
    expect(onWindowKeydown).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('passes other keydown listeners and attributes to the input, and a class to the field', async () => {
    const onKeydown = vi.fn()
    const wrapper = mountField({}, { onKeydown, autofocus: true, class: 'w-[230px]' })
    const input = wrapper.get('input')

    await input.trigger('keydown', { key: 'ArrowDown' })
    expect(onKeydown).toHaveBeenCalledOnce()
    expect(input.attributes('autofocus')).toBeDefined()
    expect(input.classes()).not.toContain('w-[230px]')
    expect(wrapper.get('label').classes()).toContain('w-[230px]')
    wrapper.unmount()
  })

  it('exposes focus() and select() for the input', () => {
    const wrapper = mountField({ modelValue: 'abc' })
    const input = wrapper.get('input').element as HTMLInputElement

    wrapper.vm.focus()
    expect(document.activeElement).toBe(input)
    wrapper.vm.select()
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, 3])
    wrapper.unmount()
  })
})
