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
  it('opens the flows canvas from the header Flows pill', async () => {
    const wrapper = mountSideBar()
    await wrapper.find('[data-testid="sidebar-open-flows"]').trigger('click')
    expect(wrapper.emitted('open-flows')).toHaveLength(1)
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
})
