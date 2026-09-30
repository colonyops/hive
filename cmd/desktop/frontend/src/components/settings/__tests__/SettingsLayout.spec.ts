import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import SettingsLayout from '../SettingsLayout.vue'

describe('SettingsLayout', () => {
  it('renders its slots and emits close on Escape', async () => {
    const wrapper = mount(SettingsLayout, {
      slots: {
        'sidebar-title': () => h('span', 'Settings title'),
        nav: () => h('button', { 'data-testid': 'settings-layout-nav' }, 'General'),
        'header-title': () => h('span', { 'data-testid': 'settings-layout-header-title' }, 'General'),
        default: () => h('main', { 'data-testid': 'settings-layout-body' }, 'Body'),
      },
    })

    expect(wrapper.text()).toContain('Settings title')
    expect(wrapper.get('[data-testid="settings-layout-nav"]').text()).toBe('General')
    expect(wrapper.get('[data-testid="settings-layout-header-title"]').text()).toBe('General')
    expect(wrapper.get('[data-testid="settings-layout-body"]').text()).toBe('Body')

    // The header carries no close button; Escape is the only affordance the
    // layout itself owns.
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
