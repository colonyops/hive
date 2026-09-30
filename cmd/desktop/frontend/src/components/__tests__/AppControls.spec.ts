import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppCheckbox from '../AppCheckbox.vue'
import AppSwitch from '../AppSwitch.vue'

describe('shared form controls', () => {
  it('updates checkbox v-model with its label and hint', async () => {
    const wrapper = mount(AppCheckbox, { props: { modelValue: false, label: 'Visible', hint: 'Show this action', testid: 'visible' } })
    await wrapper.get('[data-testid="visible"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
    expect(wrapper.text()).toContain('Show this action')
  })

  it('updates switch v-model and honors disabled state', async () => {
    const wrapper = mount(AppSwitch, { props: { modelValue: false, label: 'Enabled', testid: 'enabled' } })
    await wrapper.get('[data-testid="enabled"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
    await wrapper.setProps({ disabled: true })
    expect(wrapper.get('[data-testid="enabled"]').attributes('disabled')).toBeDefined()
  })
})
