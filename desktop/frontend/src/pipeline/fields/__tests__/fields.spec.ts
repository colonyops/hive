import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TextField from '../TextField.vue'
import SelectField from '../SelectField.vue'
import NumberField from '../NumberField.vue'
import ToggleField from '../ToggleField.vue'
import TabStrip from '../TabStrip.vue'
import GlobListField from '../GlobListField.vue'
import CodeField from '../CodeField.vue'

function fire(el: Element, type: string) {
  el.dispatchEvent(new Event(type, { bubbles: true }))
}

describe('TextField', () => {
  it('round-trips modelValue and emits update:modelValue on input', async () => {
    const wrapper = mount(TextField, { props: { modelValue: 'hello', testid: 'tf' } })
    const input = wrapper.get('input').element as HTMLInputElement
    expect(input.value).toBe('hello')

    input.value = 'world'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([['world']])
  })

  it('shows label, hint, and error text', () => {
    const wrapper = mount(TextField, { props: { modelValue: '', label: 'Name', hint: 'a hint', testid: 'tf' } })
    expect(wrapper.text()).toContain('Name')
    expect(wrapper.text()).toContain('a hint')

    const errored = mount(TextField, { props: { modelValue: '', error: 'bad value', testid: 'tf' } })
    expect(errored.text()).toContain('bad value')
    expect(errored.find('[data-testid="tf-error"]').exists()).toBe(true)
  })

  it('applies font-mono only when monospace is set', () => {
    const plain = mount(TextField, { props: { modelValue: '' } })
    expect(plain.get('input').classes()).not.toContain('font-mono')

    const mono = mount(TextField, { props: { modelValue: '', monospace: true } })
    expect(mono.get('input').classes()).toContain('font-mono')
  })
})

describe('SelectField', () => {
  it('round-trips modelValue and emits on change', async () => {
    const options = [{ value: 'a', label: 'Alpha' }, { value: 'b', label: 'Beta' }]
    const wrapper = mount(SelectField, { props: { modelValue: 'a', options, testid: 'sf' } })
    const select = wrapper.get('select').element as HTMLSelectElement
    expect(select.value).toBe('a')

    select.value = 'b'
    fire(select, 'change')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([['b']])
  })

  it('renders a disabled placeholder option when provided', () => {
    const wrapper = mount(SelectField, { props: { modelValue: '', options: [], placeholder: 'Choose one' } })
    const placeholderOption = wrapper.find('option[value=""]')
    expect(placeholderOption.exists()).toBe(true)
    expect(placeholderOption.attributes('disabled')).toBeDefined()
  })
})

describe('NumberField', () => {
  it('round-trips modelValue as a number', async () => {
    const wrapper = mount(NumberField, { props: { modelValue: 1, testid: 'nf' } })
    const input = wrapper.get('input').element as HTMLInputElement
    expect(input.value).toBe('1')

    input.value = '4'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([[4]])
  })

  it('falls back to 0 for a non-numeric value', async () => {
    const wrapper = mount(NumberField, { props: { modelValue: 1 } })
    const input = wrapper.get('input').element as HTMLInputElement
    input.value = ''
    fire(input, 'input')
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('update:modelValue')).toEqual([[0]])
  })
})

describe('ToggleField', () => {
  it('round-trips modelValue and emits the flipped boolean', async () => {
    const wrapper = mount(ToggleField, { props: { modelValue: false, testid: 'tg' } })
    const checkbox = wrapper.get('input[type="checkbox"]').element as HTMLInputElement
    expect(checkbox.checked).toBe(false)

    checkbox.checked = true
    fire(checkbox, 'change')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
  })
})

describe('TabStrip', () => {
  it('emits update:modelValue with the clicked tab value', async () => {
    const tabs = [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]
    const wrapper = mount(TabStrip, { props: { modelValue: 'a', tabs, testid: 'ts' } })

    await wrapper.get('[data-testid="ts-b"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([['b']])
  })

  it('marks the active tab aria-selected', () => {
    const tabs = [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]
    const wrapper = mount(TabStrip, { props: { modelValue: 'b', tabs } })
    const buttons = wrapper.findAll('button')
    expect(buttons[0]!.attributes('aria-selected')).toBe('false')
    expect(buttons[1]!.attributes('aria-selected')).toBe('true')
  })
})

describe('GlobListField', () => {
  it('joins modelValue array into newline-separated text', () => {
    const wrapper = mount(GlobListField, { props: { modelValue: ['a/*', 'b/*'], testid: 'gl' } })
    const textarea = wrapper.get('textarea').element as HTMLTextAreaElement
    expect(textarea.value).toBe('a/*\nb/*')
  })

  it('parses one-glob-per-line text into an array, trimming and dropping blanks', async () => {
    const wrapper = mount(GlobListField, { props: { modelValue: [], testid: 'gl' } })
    const textarea = wrapper.get('textarea').element as HTMLTextAreaElement
    textarea.value = 'acme/{a,b}/**\n  acme/cli  \n\n'
    fire(textarea, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([[['acme/{a,b}/**', 'acme/cli']]])
  })
})

describe('CodeField', () => {
  it('round-trips modelValue and emits update:modelValue on input', async () => {
    const wrapper = mount(CodeField, { props: { modelValue: 'return msg', testid: 'cf' } })
    const textarea = wrapper.get('textarea').element as HTMLTextAreaElement
    expect(textarea.value).toBe('return msg')

    textarea.value = 'return null'
    fire(textarea, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([['return null']])
  })

  it('inserts two spaces and emits an update on Tab, without moving focus', async () => {
    const wrapper = mount(CodeField, { props: { modelValue: 'ab' } })
    const textarea = wrapper.get('textarea').element as HTMLTextAreaElement
    textarea.value = 'ab'
    textarea.selectionStart = 1
    textarea.selectionEnd = 1

    await wrapper.get('textarea').trigger('keydown', { key: 'Tab' })

    expect(wrapper.emitted('update:modelValue')).toEqual([['a  b']])
  })

  it('shows an error message when provided', () => {
    const wrapper = mount(CodeField, { props: { modelValue: '', error: 'Unexpected token', testid: 'cf' } })
    expect(wrapper.find('[data-testid="cf-error"]').text()).toBe('Unexpected token')
  })

  it('renders one line-number gutter row per line, minimum one for empty content', () => {
    const empty = mount(CodeField, { props: { modelValue: '', testid: 'cf' } })
    expect(empty.get('[data-testid="cf-gutter"]').findAll('div')).toHaveLength(1)

    const wrapper = mount(CodeField, { props: { modelValue: 'a\nb\nc', testid: 'cf' } })
    const lines = wrapper.get('[data-testid="cf-gutter"]').findAll('div')
    expect(lines.map((l) => l.text())).toEqual(['1', '2', '3'])
  })

  it('renders the tokenized overlay as text — no unescaped markup from the source can reach the DOM', () => {
    const src = 'const x = "<img src=x onerror=alert(1)>"; // <script>bad</script>'
    const wrapper = mount(CodeField, { props: { modelValue: src, testid: 'cf' } })

    const pre = wrapper.get('[data-testid="cf-pre"]')
    expect(pre.text()).toBe(src)
    expect(pre.find('img').exists()).toBe(false)
    expect(pre.find('script').exists()).toBe(false)
    expect(pre.html()).toContain('&lt;script&gt;')
  })
})
