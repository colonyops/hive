import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SideBar from '../SideBar.vue'
import type { Profile } from '../../types/feed'

const profile: Profile = {
  id: 'personal',
  letter: 'P',
  name: 'Personal',
  sourceSummary: '2 sources',
  totalCount: 3,
  unreadCount: 1,
  feeds: [
    { id: 'desktop', name: 'Desktop UI', count: 2, newCount: 1 },
    { id: 'backend', name: 'Backend', count: 1, newCount: 0 },
  ],
}

function mountSideBar(overrides: Partial<{ flowsDirty: boolean }> = {}) {
  return mount(SideBar, {
    props: { profile, selection: { type: 'all' }, unreadOnly: false, ...overrides },
  })
}

describe('SideBar', () => {
  it('has no header Flows pill; a Settings gear button opens the settings page instead', async () => {
    const wrapper = mountSideBar()

    expect(wrapper.find('[data-testid="sidebar-open-flows"]').exists()).toBe(false)
    expect(wrapper.find('.flow-pill').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').text()).not.toContain('Flows')

    await wrapper.find('[data-testid="sidebar-open-settings"]').trigger('click')
    expect(wrapper.emitted('open-settings')).toHaveLength(1)
  })

  it('opens the flows canvas from the Edit flow footer', async () => {
    const wrapper = mountSideBar()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    expect(wrapper.emitted('open-flows')).toHaveLength(1)
  })

  it('selects a feed when the row is clicked', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-feed"][data-id="backend"]').trigger('click')

    expect(wrapper.emitted('select')).toEqual([[{ type: 'feed', feedId: 'backend' }]])
  })

  it('emits delete-profile from the header trash icon', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-delete-profile"]').trigger('click')

    expect(wrapper.emitted('delete-profile')).toHaveLength(1)
  })

  it('emits reveal-in-flow with the feed id from the per-feed icon, without also selecting the row', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-feed"][data-id="backend"] [data-testid="sidebar-reveal-in-flow"]').trigger('click')

    expect(wrapper.emitted('reveal-in-flow')).toEqual([['backend']])
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('shows the un-deployed changes badge only when flowsDirty is true', () => {
    const clean = mountSideBar({ flowsDirty: false })
    expect(clean.find('[data-testid="undeployed-badge"]').exists()).toBe(false)

    const dirty = mountSideBar({ flowsDirty: true })
    expect(dirty.find('[data-testid="undeployed-badge"]').exists()).toBe(true)
  })

  it('omits the un-deployed changes badge by default (flowsDirty unset)', () => {
    const wrapper = mountSideBar()
    expect(wrapper.find('[data-testid="undeployed-badge"]').exists()).toBe(false)
  })
})
