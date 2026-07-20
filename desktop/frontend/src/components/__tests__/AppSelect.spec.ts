import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppSelect from '../AppSelect.vue'

const options = [
  { value: 'launch-session', label: 'Launch session' },
  { value: 'shell', label: 'Shell' },
]

describe('AppSelect', () => {
  it('shows the selected label, opens on click, and emits the chosen value', async () => {
    const wrapper = mount(AppSelect, { props: { modelValue: 'launch-session', options, testid: 'action-type' } })
    expect(wrapper.get('[data-testid="action-type"]').text()).toContain('Launch session')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)

    await wrapper.get('[data-testid="action-type"]').trigger('click')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)

    await wrapper.get('[data-testid="action-type-option-shell"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['shell'])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('does not re-emit when the current value is re-selected', async () => {
    const wrapper = mount(AppSelect, { props: { modelValue: 'shell', options, testid: 'action-type' } })
    await wrapper.get('[data-testid="action-type"]').trigger('click')
    await wrapper.get('[data-testid="action-type-option-shell"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('selects the active option with the keyboard', async () => {
    const wrapper = mount(AppSelect, { props: { modelValue: 'launch-session', options, testid: 'action-type' } })
    const root = wrapper.get('[data-testid="action-type"]')
    await root.trigger('keydown', { key: 'ArrowDown' }) // open
    await root.trigger('keydown', { key: 'ArrowDown' }) // move to shell
    await root.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['shell'])
  })
})
