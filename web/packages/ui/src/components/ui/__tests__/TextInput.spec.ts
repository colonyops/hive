import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import TextArea from '../TextArea.vue'
import TextInput from '../TextInput.vue'

describe('TextInput', () => {
  it('shows modelValue and emits each edit', async () => {
    const wrapper = mount(TextInput, { props: { modelValue: 'old' } })
    const input = wrapper.get('input')

    expect((input.element as HTMLInputElement).value).toBe('old')
    await input.setValue('new')
    expect(wrapper.emitted('update:modelValue')).toEqual([['new']])
  })

  it('emits a string for a number input', async () => {
    const wrapper = mount(TextInput, { props: { modelValue: '1', type: 'number' } })

    await wrapper.get('input').setValue('42')
    expect(wrapper.emitted('update:modelValue')).toEqual([['42']])
  })

  it('passes attributes and listeners through to the native input', async () => {
    const onKeydown = vi.fn()
    const wrapper = mount(TextInput, {
      attrs: { id: 'name', placeholder: 'Name', 'data-testid': 'thing-name', autocomplete: 'off', onKeydown },
    })
    const input = wrapper.get('input')

    expect(input.attributes()).toMatchObject({
      id: 'name',
      placeholder: 'Name',
      'data-testid': 'thing-name',
      autocomplete: 'off',
    })
    await input.trigger('keydown', { key: 'Enter' })
    expect(onKeydown).toHaveBeenCalledOnce()
  })

  it('marks itself invalid for assistive tech', async () => {
    const wrapper = mount(TextInput)
    expect(wrapper.get('input').attributes('aria-invalid')).toBeUndefined()

    await wrapper.setProps({ invalid: true })
    expect(wrapper.get('input').attributes('aria-invalid')).toBe('true')
  })

  it('exposes focus() and select() for the native input', () => {
    const wrapper = mount(TextInput, { props: { modelValue: 'draft' }, attachTo: document.body })
    const input = wrapper.get('input').element as HTMLInputElement

    wrapper.vm.focus()
    expect(document.activeElement).toBe(input)
    wrapper.vm.select()
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, 5])
    wrapper.unmount()
  })
})

describe('TextArea', () => {
  it('shows modelValue and emits each edit', async () => {
    const wrapper = mount(TextArea, { props: { modelValue: 'a', rows: 5 } })
    const textarea = wrapper.get('textarea')

    expect((textarea.element as HTMLTextAreaElement).value).toBe('a')
    expect(textarea.attributes('rows')).toBe('5')
    await textarea.setValue('a\nb')
    expect(wrapper.emitted('update:modelValue')).toEqual([['a\nb']])
  })

  it('exposes focus() for the native textarea', () => {
    const wrapper = mount(TextArea, { attachTo: document.body })

    wrapper.vm.focus()
    expect(document.activeElement).toBe(wrapper.get('textarea').element)
    wrapper.unmount()
  })
})
