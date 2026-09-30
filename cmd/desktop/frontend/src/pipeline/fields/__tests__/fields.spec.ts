import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TextField from '../TextField.vue'
import TextareaField from '../TextareaField.vue'
import SelectField from '../SelectField.vue'
import NumberField from '../NumberField.vue'
import ToggleField from '../ToggleField.vue'
import GlobListField from '../GlobListField.vue'
import CodeField from '../CodeField.vue'
import { chooseOption, openSelect } from '../../../test-utils/select'

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

describe('TextareaField', () => {
  it('renders its label and forwards testid, rows, and monospace styling to the textarea', () => {
    const wrapper = mount(TextareaField, { props: { modelValue: 'hello', label: 'Template', testid: 'ta', rows: 5, monospace: true } })
    const textarea = wrapper.get('[data-testid="ta"]').element as HTMLTextAreaElement

    expect(wrapper.text()).toContain('Template')
    expect(textarea.value).toBe('hello')
    expect(textarea.getAttribute('rows')).toBe('5')
    expect(textarea.classList).toContain('font-mono')
  })

  it('emits update:modelValue on input', async () => {
    const wrapper = mount(TextareaField, { props: { modelValue: '', testid: 'ta' } })
    const textarea = wrapper.get('textarea').element as HTMLTextAreaElement
    textarea.value = 'updated'
    fire(textarea, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:modelValue')).toEqual([['updated']])
  })
})

describe('SelectField', () => {
  const options = [{ value: 'a', label: 'Alpha' }, { value: 'b', label: 'Beta' }]

  it('round-trips modelValue and emits the chosen option', async () => {
    const wrapper = mount(SelectField, { props: { modelValue: 'a', options, testid: 'sf' } })
    expect(wrapper.get('[data-testid="sf"]').text()).toContain('Alpha')

    await chooseOption(wrapper, 'sf', 'b')

    expect(wrapper.emitted('update:modelValue')).toEqual([['b']])
  })

  it('shows the placeholder when nothing is selected', () => {
    const wrapper = mount(SelectField, { props: { modelValue: '', options: [], placeholder: 'Choose one', testid: 'sf' } })
    expect(wrapper.get('[data-testid="sf"]').text()).toContain('Choose one')
  })

  it('wraps the control in FieldRow chrome and labels it', () => {
    const wrapper = mount(SelectField, { props: { modelValue: 'a', options, label: 'Kind', hint: 'pick one', testid: 'sf' } })
    expect(wrapper.text()).toContain('Kind')
    expect(wrapper.get('[data-testid="sf-hint"]').text()).toBe('pick one')
    expect(wrapper.get('[data-testid="sf"]').attributes('aria-label')).toBe('Kind')
  })

  it('offers a search box only when searchable', async () => {
    const plain = mount(SelectField, { props: { modelValue: 'a', options, testid: 'sf' } })
    expect((await openSelect(plain, 'sf')).querySelector('[data-testid="sf-search"]')).toBeNull()
    plain.unmount()

    const searchable = mount(SelectField, { props: { modelValue: 'a', options, searchable: true, testid: 'sf' } })
    expect((await openSelect(searchable, 'sf')).querySelector('[data-testid="sf-search"]')).not.toBeNull()
    searchable.unmount()
  })

  it('does not open when disabled', async () => {
    const wrapper = mount(SelectField, { props: { modelValue: 'a', options, disabled: true, testid: 'sf' } })
    await wrapper.get('[data-testid="sf"]').trigger('click')
    expect(document.querySelector('[data-testid="sf-popover"]')).toBeNull()
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
