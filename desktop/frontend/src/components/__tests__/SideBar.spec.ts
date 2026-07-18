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

function mountSideBar() {
  return mount(SideBar, {
    props: { profile, selection: { type: 'all' }, unreadOnly: false },
  })
}

describe('SideBar', () => {
  it('emits edit-feed with the feed id from the row pencil, without selecting the row', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-feed-edit-desktop"]').trigger('click')

    expect(wrapper.emitted('edit-feed')).toEqual([['desktop']])
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('renders one pencil per feed row', () => {
    const wrapper = mountSideBar()

    expect(wrapper.find('[data-testid="sidebar-feed-edit-desktop"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="sidebar-feed-edit-backend"]').exists()).toBe(true)
  })

  it('still selects a feed when the row itself is clicked', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-feed"][data-id="backend"]').trigger('click')

    expect(wrapper.emitted('select')).toEqual([[{ type: 'feed', feedId: 'backend' }]])
    expect(wrapper.emitted('edit-feed')).toBeUndefined()
  })

  it('keeps the FEEDS header button emitting edit-feeds', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-edit-feeds"]').trigger('click')

    expect(wrapper.emitted('edit-feeds')).toHaveLength(1)
  })

  it('emits delete-profile from the header trash icon', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-delete-profile"]').trigger('click')

    expect(wrapper.emitted('delete-profile')).toHaveLength(1)
  })
})
