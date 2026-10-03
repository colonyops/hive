import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { mount } from '@vue/test-utils'
import FormField from '../FormField.vue'

function mountWithInput(props: Record<string, unknown> = {}) {
  return mount(FormField, {
    props: { label: 'Name', testid: 'thing', ...props },
    slots: { default: ({ id }: { id: string }) => h('input', { id, 'data-testid': 'thing-input' }) },
  })
}

describe('FormField', () => {
  it('points its label at the id it hands the slot', () => {
    const wrapper = mountWithInput()

    const id = wrapper.get('[data-testid="thing-input"]').attributes('id')
    expect(id).toBeTruthy()
    expect(wrapper.get('label').attributes('for')).toBe(id)
    expect(wrapper.get('label').text()).toBe('Name')
  })

  it('gives each field in a form its own id', () => {
    const wrapper = mount({ render: () => h('form', [h(FormField, { label: 'A' }), h(FormField, { label: 'B' })]) })

    const [first, second] = wrapper.findAll('label').map((label) => label.attributes('for'))
    expect(first).not.toBe(second)
  })

  it('shows the hint under a derived test id', () => {
    const wrapper = mountWithInput({ hint: 'Pick a name' })

    expect(wrapper.get('[data-testid="thing-hint"]').text()).toBe('Pick a name')
    expect(wrapper.find('[data-testid="thing-error"]').exists()).toBe(false)
  })

  it('shows the error as an alert in place of the hint', () => {
    const wrapper = mountWithInput({ hint: 'Pick a name', error: 'Name is taken' })

    const error = wrapper.get('[data-testid="thing-error"]')
    expect(error.text()).toBe('Name is taken')
    expect(error.attributes('role')).toBe('alert')
    expect(wrapper.find('[data-testid="thing-hint"]').exists()).toBe(false)
  })

  it('accepts a null error', () => {
    const wrapper = mountWithInput({ hint: 'Pick a name', error: null })

    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="thing-hint"]').exists()).toBe(true)
  })

  it('renders a label slot in place of the label prop', () => {
    const wrapper = mount(FormField, { slots: { label: 'Prompt <span>(optional)</span>' } })

    expect(wrapper.get('label').text()).toBe('Prompt (optional)')
  })

  it('renders no label when given none', () => {
    const wrapper = mount(FormField, { slots: { default: '<input />' } })

    expect(wrapper.find('label').exists()).toBe(false)
  })
})
