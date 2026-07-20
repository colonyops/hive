import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedList from '../FeedList.vue'
import type { FeedItem } from '../../types/feed'

function item(id: string, title: string, unread: boolean): FeedItem {
  return {
    id,
    kind: 'PR',
    repo: 'colonyops/hive',
    num: 1,
    title,
    author: 'hayden',
    age: '5m',
    unread,
    feedId: 'triage/desktop',
    labels: [],
    branch: 'feat/desktop-ui-shell',
    body: 'Body',
    prompt: 'Prompt',
    url: 'https://github.com/colonyops/hive/pull/42',
  }
}

const items = [item('unread-1', 'Unread item', true), item('read-1', 'Read item', false)]

// The store owns filtering now, so FeedList renders exactly the `visibleItems`
// it is given; these props describe the pre-filtered view.
function mountList(overrides: Partial<{
  title: string
  visibleItems: FeedItem[]
  selectedId: string | null
  unreadOnly: boolean
  unreadCount: number
  search: string
  loadError: string | null
}> = {}) {
  return mount(FeedList, {
    props: {
      title: 'All items',
      visibleItems: items,
      selectedId: null,
      unreadOnly: false,
      unreadCount: 1,
      search: '',
      loadError: null,
      ...overrides,
    },
  })
}

describe('FeedList', () => {
  it('renders the visible items it is given', () => {
    const wrapper = mountList({ visibleItems: [item('unread-1', 'Unread item', true)] })
    expect(wrapper.text()).toContain('Unread item')
    expect(wrapper.text()).not.toContain('Read item')
  })

  it('shows the unread count from its prop', () => {
    const wrapper = mountList({ unreadCount: 3 })
    expect(wrapper.find('[data-testid="filter-unread"]').text()).toContain('3')
  })

  it('emits select with the clicked item id', async () => {
    const wrapper = mountList()
    const itemButton = wrapper.findAll('button.feed-item').find((button) => button.text().includes('Read item'))

    expect(itemButton).toBeTruthy()
    await itemButton!.trigger('click')

    expect(wrapper.emitted('select')).toEqual([['read-1']])
  })

  it('emits update:search as the search box changes', async () => {
    const wrapper = mountList()

    await wrapper.find('[data-testid="feed-search"]').setValue('oauth')

    expect(wrapper.emitted('update:search')).toEqual([['oauth']])
  })

  it('shows the unreachable state with a retry that emits refresh', async () => {
    const wrapper = mountList({ visibleItems: [], loadError: "Can't reach GitHub right now." })

    const error = wrapper.get('[data-testid="feed-error"]')
    expect(error.text()).toContain('GitHub unreachable')
    expect(error.text()).toContain("Can't reach GitHub right now.")

    await error.get('button').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('shows the all-caught-up state when the unread filter drains the list', () => {
    const wrapper = mountList({ visibleItems: [], unreadOnly: true, unreadCount: 0, title: 'Unread' })
    expect(wrapper.get('[data-testid="feed-empty"]').text()).toContain("You're all caught up")
  })

  it('shows the plain empty state without the unread filter', () => {
    const wrapper = mountList({ visibleItems: [], unreadCount: 0 })
    expect(wrapper.get('[data-testid="feed-empty"]').text()).toContain('No items yet')
  })

  it('shows a no-matches empty state when the search excludes everything', () => {
    const wrapper = mountList({ visibleItems: [], search: 'zzz-nothing-here' })
    expect(wrapper.get('[data-testid="feed-empty"]').text()).toContain('No matches')
  })

  it('emits explicit unread filter values and refresh from header controls', async () => {
    const wrapper = mountList()

    await wrapper.find('[data-testid="filter-unread"]').trigger('click')
    await wrapper.find('[data-testid="filter-all"]').trigger('click')
    await wrapper.find('[data-testid="refresh-chip"]').trigger('click')

    expect(wrapper.emitted('set-unread')).toEqual([[true], [false]])
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })
})
