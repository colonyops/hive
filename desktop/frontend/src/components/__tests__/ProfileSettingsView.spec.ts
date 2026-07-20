import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProfileSettingsView from '../ProfileSettingsView.vue'

const profile = {
  id: 'personal',
  letter: 'P',
  name: 'Personal',
  sourceSummary: 'GitHub · 2 sources',
  totalCount: 3,
  unreadCount: 1,
  feeds: [],
}

describe('ProfileSettingsView', () => {
  it('shows profile details and keeps delete in the danger zone', async () => {
    const wrapper = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })

    expect(wrapper.find('[data-testid="profile-settings-name"]').text()).toBe('Personal')
    expect(wrapper.find('[data-testid="profile-settings-delete"]').exists()).toBe(false)

    await wrapper.find('[data-testid="profile-settings-danger"]').trigger('click')
    expect(wrapper.emitted('select-section')).toEqual([['danger']])
    await wrapper.setProps({ activeSection: 'danger' })
    await wrapper.find('[data-testid="profile-settings-delete"]').trigger('click')

    expect(wrapper.emitted('delete')).toHaveLength(1)
  })

  it('closes from the header action', async () => {
    const wrapper = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })
    await wrapper.find('[data-testid="profile-settings-close"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
