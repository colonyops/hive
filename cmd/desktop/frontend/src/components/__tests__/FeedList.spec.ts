import { describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import FeedList from '../FeedList.vue'
import type { ActionView } from '../../types/action'
import type { InboxItem } from '../../types/feed'

function item(id: number, title: string, unread = false): InboxItem {
  return {
    id,
    profileId: 'triage',
    sourceKind: 'github',
    sourceScope: 'acme/app',
    externalId: `pr-${id}`,
    title,
    url: '',
    payload: { kind: 'PR', repo: 'acme/app', num: id, author: 'hay', body: 'Body' },
    revision: 1,
    unread,
    lifecycle: 'active',
    firstSeenAt: 1,
    lastEventAt: Date.now(),
  }
}

// A year back so the tier is "Older" whatever the suite's wall clock says —
// nearer ages straddle the month tiers depending on the day of the month.
const day = 24 * 60 * 60 * 1000
const longAgo = 400 * day
const aged = (id: number, ageMs: number): InboxItem => ({ ...item(id, `Item ${id}`), lastEventAt: Date.now() - ageMs })
const dividerLabels = (wrapper: VueWrapper) =>
  wrapper.findAll('[data-testid="feed-date-label"]').map((label) => label.text())

function mountList(
  overrides: Partial<{
    visibleItems: InboxItem[]
    archivedItems: InboxItem[]
    archivedCount: number
    archivedExpanded: boolean
    trash: boolean
    trashFilter: 'all' | 'ignored'
    selectedId: number | null
    unreadOnly: boolean
    unreadCount: number
    search: string
    authors: string[]
    authorFilter: string
    sort: 'newest' | 'oldest' | 'unread'
    loadError: string | null
    refreshing: boolean
    selectionMode: boolean
    selectedItemIds: number[]
    selectionActions: ActionView[]
  }> = {},
) {
  return mount(FeedList, {
    props: {
      title: 'Feed',
      visibleItems: [item(1, 'Unread', true), item(2, 'Read')],
      archivedItems: [],
      archivedCount: 0,
      archivedExpanded: false,
      trash: false,
      trashFilter: 'all',
      selectedId: null,
      unreadOnly: false,
      unreadCount: 1,
      search: '',
      authors: [],
      authorFilter: '',
      sort: 'newest',
      loadError: null,
      refreshing: false,
      selectionMode: false,
      selectedItemIds: [],
      selectionActions: [],
      ...overrides,
    },
  })
}

describe('FeedList', () => {
  it('renders the supplied inbox rows and emits their numeric inbox ids', async () => {
    const wrapper = mountList()
    expect(wrapper.text()).toContain('Unread')
    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('click')
    expect(wrapper.emitted('select')).toEqual([[2]])
  })

  it('separates selecting a row from activating it', async () => {
    const wrapper = mountList()
    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('dblclick')
    expect(wrapper.emitted('activate')).toEqual([[2]])
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('renders the unread count and changes the list-level unread filter', async () => {
    const wrapper = mountList({ unreadCount: 3 })
    expect(wrapper.get('[data-testid="filter-unread"]').text()).toContain('3')
    await wrapper.get('[data-testid="filter-unread"]').trigger('click')
    await wrapper.get('[data-testid="filter-all"]').trigger('click')
    expect(wrapper.emitted('set-unread')).toEqual([[true], [false]])
  })

  it('uses an icon-only horizontal ellipsis for feed options', () => {
    const toggle = mountList().get('[data-testid="view-menu-toggle"]')
    expect(toggle.attributes('aria-label')).toBe('Feed options')
    expect(toggle.text()).toBe('')
  })

  it('emits sort and refresh choices from the feed options menu', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="view-sort-oldest"]').trigger('click')
    expect(wrapper.emitted('set-sort')).toEqual([['oldest']])

    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="view-menu-refresh"]').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('offers author filtering only when authors are available', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="view-author-filter"]').exists()).toBe(false)
    await wrapper.setProps({ authors: ['amy', 'ben'] })
    await wrapper.get('[data-testid="view-author-filter"]').trigger('click')
    expect(wrapper.find('[data-testid="view-author-submenu"]').exists()).toBe(true)
    await wrapper.get('[data-testid="view-author-search"]').setValue('am')
    expect(wrapper.findAll('[data-testid="view-author-option"]')).toHaveLength(1)
    await wrapper.get('[data-testid="view-author-option"]').trigger('click')
    expect(wrapper.emitted('update:author-filter')).toEqual([['amy']])
    expect(wrapper.find('[data-testid="view-menu"]').exists()).toBe(false)
    await wrapper.setProps({ authorFilter: 'amy' })
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="view-author-submenu"]').exists()).toBe(false)
    await wrapper.get('[data-testid="view-author-filter"]').trigger('click')
    expect(wrapper.findAll('[data-testid="view-author-option"]')[0]!.attributes('aria-checked')).toBe('true')
    await wrapper.get('[data-testid="view-author-all"]').trigger('click')
    expect(wrapper.emitted('update:author-filter')).toEqual([['amy'], ['']])
  })

  it('keeps a missing author filter visible and clearable and explains empty results', async () => {
    const wrapper = mountList({ authorFilter: 'amy', visibleItems: [], unreadOnly: true })
    expect(wrapper.get('[data-testid="feed-empty"]').text()).toContain('No items by amy match the current filters')
    expect(wrapper.get('[data-testid="feed-empty"]').text()).not.toContain("You're all caught up")
    await wrapper.get('[data-testid="clear-author-filter"]').trigger('click')
    expect(wrapper.emitted('update:author-filter')).toEqual([['']])
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="view-author-filter"]').trigger('click')
    expect(wrapper.find('[data-testid="view-author-all"]').exists()).toBe(true)
  })

  it('offers mark all as read in the view menu and closes after', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')

    const entry = wrapper.get('[data-testid="view-menu-mark-read"]')
    expect(entry.text()).toBe('Mark all as read') // no shortcut glyph in the label

    await entry.trigger('click')
    expect(wrapper.emitted('mark-all-read')).toHaveLength(1)
    expect(wrapper.find('[data-testid="view-menu"]').exists()).toBe(false)
  })

  it('omits mark all as read in trash, which carries no unread semantics', async () => {
    const wrapper = mountList({ trash: true })
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="view-menu-mark-read"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="view-menu-refresh"]').exists()).toBe(true)
  })

  it('enters selection mode from feed options and emits selection actions', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="view-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="view-menu-select-items"]').trigger('click')
    expect(wrapper.emitted('enter-selection')).toHaveLength(1)

    await wrapper.setProps({ selectionMode: true, selectedItemIds: [1, 2] })
    expect(wrapper.get('.selection-count').attributes('aria-label')).toBe('2 selected')
    expect(wrapper.get('[data-testid="selection-copy-contents"]').attributes('title')).toBe('Copy contents')
    expect(wrapper.get('[data-testid="selection-create-session"]').attributes('title')).toBe('Create session')
    await wrapper.get('[data-testid="selection-copy-contents"]').trigger('click')
    await wrapper.get('[data-testid="selection-create-session"]').trigger('click')
    await wrapper.get('[data-testid="selection-cancel"]').trigger('click')
    expect(wrapper.emitted('copy-selection-contents')).toHaveLength(1)
    expect(wrapper.emitted('create-session-from-selection')).toHaveLength(1)
    expect(wrapper.emitted('cancel-selection')).toHaveLength(1)
  })

  it('offers clipboard actions that apply to the whole selection', async () => {
    const wrapper = mountList({
      selectionMode: true,
      selectedItemIds: [1, 2],
      selectionActions: [
        {
          id: 'copy-checkout',
          label: 'Copy checkout commands',
          type: 'clipboard',
          showInDetail: true,
          requiresSessionInput: false,
        },
      ],
    })

    await wrapper.get('[data-testid="selection-actions-toggle"]').trigger('click')
    await wrapper.get('[data-testid="selection-action-copy-checkout"]').trigger('click')

    expect(wrapper.emitted('run-selection-action')).toEqual([['copy-checkout']])
    expect(wrapper.find('[data-testid="selection-actions-menu"]').exists()).toBe(false)
  })

  it('shows refresh progress below the search controls', () => {
    const wrapper = mountList({ refreshing: true })
    expect(wrapper.get('[data-testid="feed-refreshing"]').text()).toContain('Refreshing')
    expect(wrapper.find('[data-testid="header-refresh-spinner"]').exists()).toBe(false)
  })

  it('relays search input without owning filtering', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="feed-search"]').setValue('oauth')
    expect(wrapper.emitted('update:search')).toEqual([['oauth']])
  })

  // view.focus-search reaches this through the exposed handle rather than a
  // prop, since it fires from the global keymap rather than a click. Selected,
  // not just focused: a repeat press means "start a new search" far more often
  // than "edit the old one".
  it('selects existing search text via the exposed focusSearch handle', () => {
    const wrapper = mountList({ search: 'oauth' })
    const input = wrapper.get('[data-testid="feed-search"]').element as HTMLInputElement
    const select = vi.spyOn(input, 'select')

    ;(wrapper.vm as unknown as { focusSearch: () => void }).focusSearch()

    expect(select).toHaveBeenCalled()
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
    expect(mountList({ visibleItems: [], unreadOnly: true }).get('[data-testid="feed-empty"]').text()).toContain(
      "You're all caught up",
    )
    expect(mountList({ visibleItems: [], search: 'none' }).get('[data-testid="feed-empty"]').text()).toContain(
      'No matches',
    )
    expect(mountList({ loadError: 'offline' }).get('[data-testid="feed-error"]').text()).toContain('offline')
  })

  it('separates rows into date tiers and leaves a leading Today unlabeled', () => {
    const wrapper = mountList({ visibleItems: [aged(1, 0), aged(2, longAgo), aged(3, longAgo)] })
    expect(dividerLabels(wrapper)).toEqual(['Older'])
    // The strip counts the rows in its own tier, not the whole list.
    expect(wrapper.get('[data-testid="feed-date-count"]').text()).toBe('2')
    // Every row still renders — suppressing the label drops the separator, not the group.
    expect(wrapper.findAll('[data-testid="feed-item"]')).toHaveLength(3)
  })

  it('labels Today when oldest-first sort moves it to the bottom', () => {
    const wrapper = mountList({ sort: 'oldest', visibleItems: [aged(2, longAgo), aged(1, 0)] })
    expect(dividerLabels(wrapper)).toEqual(['Older', 'Today'])
  })

  it('drops the date separators under unread-first sort, which interleaves dates', () => {
    const wrapper = mountList({ sort: 'unread' })
    expect(wrapper.find('[data-testid="feed-date-divider"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="feed-item"]')).toHaveLength(2)
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
