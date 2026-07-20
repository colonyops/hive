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
    props: { profile, selection: { type: 'all' }, ...overrides },
  })
}

describe('SideBar', () => {
  it('has no header Flows pill or delete action; the gear opens profile settings', async () => {
    const wrapper = mountSideBar()

    expect(wrapper.find('[data-testid="sidebar-open-flows"]').exists()).toBe(false)
    expect(wrapper.find('.flow-pill').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').text()).not.toContain('Flows')
    expect(wrapper.find('[data-testid="sidebar-delete-profile"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-open-settings"]').attributes('aria-label')).toBe('Profile settings')

    await wrapper.find('[data-testid="sidebar-open-settings"]').trigger('click')
    expect(wrapper.emitted('open-settings')).toHaveLength(1)
  })

  it('no longer carries an Unread shortcut (the All / Unread filter lives on the list)', () => {
    const wrapper = mountSideBar()
    expect(wrapper.find('[data-testid="sidebar-unread"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Unread')
  })

  it('highlights All items whenever the scope is all-items', () => {
    const wrapper = mountSideBar()
    const allEntry = wrapper.findAll('button.sidebar-entry').find((b) => b.text().includes('All items'))
    expect(allEntry?.classes()).toContain('sidebar-entry-selected')
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

  it('renders a resize handle that widens the panel on drag and persists the width', async () => {
    const wrapper = mountSideBar()
    const aside = wrapper.get('aside').element as HTMLElement
    expect(aside.style.width).toBe('250px') // default

    const handle = wrapper.get('[data-testid="resize-handle-sidebar"]')
    expect(handle.attributes('role')).toBe('separator')

    await handle.trigger('pointerdown', { clientX: 250, pointerId: 1 })
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 300, pointerId: 1 }))
    await wrapper.vm.$nextTick()

    expect(aside.style.width).toBe('300px')

    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 300, pointerId: 1 }))
    expect(localStorage.getItem('hive.panel.sidebar')).toBe('300')
  })
})
