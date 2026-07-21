import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SettingsNavItem from '../SettingsNavItem.vue'

describe('SettingsNavItem', () => {
  it('applies active and danger styling', async () => {
    const wrapper = mount(SettingsNavItem, {
      props: { active: true, label: 'Danger zone' },
    })

    expect(wrapper.classes()).toContain('bg-hover')
    expect(wrapper.classes()).toContain('font-medium')
    expect(wrapper.classes()).toContain('text-accent')
    expect(wrapper.attributes('aria-current')).toBe('true')

    await wrapper.setProps({ tone: 'danger' })

    expect(wrapper.classes()).toContain('text-severity-error')
    expect(wrapper.classes()).not.toContain('text-accent')

    await wrapper.setProps({ active: false })

    expect(wrapper.classes()).toContain('text-text-2')
    expect(wrapper.classes()).not.toContain('text-severity-error')
    expect(wrapper.attributes('aria-current')).toBeUndefined()
  })

  it('emits select and preserves its test id', async () => {
    const wrapper = mount(SettingsNavItem, {
      props: { active: false, label: 'General', testid: 'settings-nav-general' },
    })

    expect(wrapper.attributes('data-testid')).toBe('settings-nav-general')
    await wrapper.trigger('click')

    expect(wrapper.emitted('select')).toHaveLength(1)
  })
})
