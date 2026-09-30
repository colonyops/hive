import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import BaseButton from '../BaseButton.vue'

describe('BaseButton', () => {
  it.each([
    ['primary', 'bg-accent'],
    ['secondary', 'border-card'],
    ['danger', 'bg-severity-error'],
    ['ghost', 'text-text-2'],
  ] as const)('applies %s variant classes', (variant, className) => {
    const wrapper = mount(BaseButton, { props: { variant } })
    expect(wrapper.classes()).toContain(className)
  })

  it('disables while busy and honors submit type', () => {
    const wrapper = mount(BaseButton, { props: { busy: true, type: 'submit' } })
    expect(wrapper.attributes('disabled')).toBeDefined()
    expect(wrapper.attributes('type')).toBe('submit')
  })

  it('emits clicks and forwards data attributes', async () => {
    const wrapper = mount(BaseButton, { attrs: { 'data-testid': 'save-button', 'aria-label': 'Save changes' } })
    await wrapper.trigger('click')

    expect(wrapper.emitted('click')).toHaveLength(1)
    expect(wrapper.attributes('data-testid')).toBe('save-button')
    expect(wrapper.attributes('aria-label')).toBe('Save changes')
  })

  it('renders a leading icon slot before its label', () => {
    const wrapper = mount(BaseButton, {
      slots: {
        icon: () => h('span', { 'data-testid': 'button-icon' }, 'icon'),
        default: () => 'Save',
      },
    })

    expect(wrapper.find('[data-testid="button-icon"]').exists()).toBe(true)
    expect(wrapper.text()).toBe('iconSave')
  })
})
