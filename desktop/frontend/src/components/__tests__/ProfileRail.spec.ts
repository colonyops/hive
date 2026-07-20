import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProfileRail from '../ProfileRail.vue'

const profiles = [{
  id: 'personal',
  letter: 'P',
  name: 'Personal',
  sourceSummary: 'GitHub · 2 sources',
  totalCount: 3,
  unreadCount: 1,
  feeds: [],
}]

describe('ProfileRail', () => {
  it('shows one application settings action at the bottom', async () => {
    const wrapper = mount(ProfileRail, { props: { profiles, activeProfileId: 'personal' } })

    expect(wrapper.find('[data-testid="application-settings"]').attributes('aria-label')).toBe('Application settings')
    expect(wrapper.text()).not.toContain('hy')

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    expect(wrapper.emitted('open-settings')).toHaveLength(1)
  })
})
