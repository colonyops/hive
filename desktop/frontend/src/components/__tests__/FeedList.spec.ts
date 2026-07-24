import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedList from '../FeedList.vue'
import type { InboxItem } from '../../types/feed'

function item(id: number, title: string, unread = false): InboxItem {
  return { id, profileId: 'triage', sourceKind: 'github', sourceScope: 'acme/app', externalId: `pr-${id}`, title, url: '', payload: { kind: 'PR', repo: 'acme/app', num: id, author: 'hay', body: 'Body' }, revision: 1, unread, lifecycle: 'active', firstSeenAt: 1, lastEventAt: Date.now() }
}

function mountList(overrides: Partial<{ visibleItems: InboxItem[]; archivedItems: InboxItem[]; archivedCount: number; archivedExpanded: boolean; trash: boolean; trashFilter: 'all' | 'ignored'; selectedId: number | null; unreadOnly: boolean; unreadCount: number; search: string; sort: 'newest' | 'oldest' | 'unread'; loadError: string | null }> = {}) {
  return mount(FeedList, { props: { title: 'Feed', visibleItems: [item(1, 'Unread', true), item(2, 'Read')], archivedItems: [], archivedCount: 0, archivedExpanded: false, trash: false, trashFilter: 'all', selectedId: null, unreadOnly: false, unreadCount: 1, search: '', sort: 'newest', loadError: null, ...overrides } })
}

describe('FeedList', () => {
  it('renders the supplied inbox rows and emits their numeric inbox ids', async () => {
    const wrapper = mountList()
    expect(wrapper.text()).toContain('Unread')
    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('click')
    expect(wrapper.emitted('select')).toEqual([[2]])
  })

  it('renders the unread count and changes the list-level unread filter', async () => {
    const wrapper = mountList({ unreadCount: 3 })
    expect(wrapper.get('[data-testid="filter-unread"]').text()).toContain('3')
    await wrapper.get('[data-testid="filter-unread"]').trigger('click')
    await wrapper.get('[data-testid="filter-all"]').trigger('click')
    expect(wrapper.emitted('set-unread')).toEqual([[true], [false]])
  })

  it('emits sort and refresh choices from the view menu', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="view-sort-oldest"]').trigger('click')
    expect(wrapper.emitted('set-sort')).toEqual([['oldest']])

    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="view-menu-refresh"]').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('relays search input without owning filtering', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="feed-search"]').setValue('oauth')
    expect(wrapper.emitted('update:search')).toEqual([['oauth']])
  })

  it('shows a collapsed archived divider and expands it on demand', async () => {
    const wrapper = mountList({ archivedCount: 2 })
    const divider = wrapper.get('[data-testid="archived-divider"]')
    expect(divider.text()).toContain('Archived (2)')
    expect(divider.attributes('aria-expanded')).toBe('false')
    await divider.trigger('click')
    expect(wrapper.emitted('toggle-archived')).toHaveLength(1)
  })

  it('renders archived rows below active rows when expanded', () => {
    const archived = { ...item(9, 'Done'), archivedAt: 5, archivedReason: 'merged' }
    const wrapper = mountList({ archivedCount: 1, archivedExpanded: true, archivedItems: [archived] })
    const rows = wrapper.findAll('[data-testid="feed-item"]')
    expect(rows).toHaveLength(3)
    expect(rows[2]!.text()).toContain('Done')
    expect(wrapper.find('[data-testid="archive-reason"]').text()).toBe('merged')
  })

  it('hides the archived divider and unread filter in trash, offering the ignored filter instead', async () => {
    const ignored = { ...item(4, 'Muted'), ignoredAt: 7 }
    const wrapper = mountList({ trash: true, archivedCount: 3, visibleItems: [ignored] })
    expect(wrapper.find('[data-testid="archived-divider"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="filter-unread"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ignored-pill"]').text()).toBe('ignored')
    await wrapper.get('[data-testid="filter-trash-ignored"]').trigger('click')
    expect(wrapper.emitted('set-trash-filter')).toEqual([['ignored']])
  })

  it('shows distinct empty, unread-drained, search, and load-error states', () => {
    expect(mountList({ visibleItems: [] }).get('[data-testid="feed-empty"]').text()).toContain('No items yet')
    expect(mountList({ visibleItems: [], unreadOnly: true }).get('[data-testid="feed-empty"]').text()).toContain("You're all caught up")
    expect(mountList({ visibleItems: [], search: 'none' }).get('[data-testid="feed-empty"]').text()).toContain('No matches')
    expect(mountList({ loadError: 'offline' }).get('[data-testid="feed-error"]').text()).toContain('offline')
  })

  it('anchors keyboard selection by numeric inbox id rather than external source id', async () => {
    const scrollIntoView = vi.fn()
    vi.spyOn(HTMLElement.prototype, 'scrollIntoView').mockImplementation(scrollIntoView)
    const wrapper = mountList()
    await wrapper.setProps({ selectedId: 2 })
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('[data-testid="feed-item"]')[1]!.attributes('data-inbox-id')).toBe('2')
    expect(scrollIntoView).toHaveBeenCalledWith({ block: 'nearest' })
  })
})
