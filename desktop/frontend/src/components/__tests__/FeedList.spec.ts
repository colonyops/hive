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

function mountList(unreadOnly = false, loadError: string | null = null) {
  return mount(FeedList, {
    props: {
      title: 'All items',
      items,
      selectedId: null,
      unreadOnly,
      countLabel: '2 · 1 unread',
      loadError,
    },
  })
}

describe('FeedList', () => {
  it('hides read items when unreadOnly is true', () => {
    const wrapper = mountList(true)
    expect(wrapper.text()).toContain('Unread item')
    expect(wrapper.text()).not.toContain('Read item')
  })

  it('emits select with the selected item id', async () => {
    const wrapper = mountList()
    const itemButton = wrapper.findAll('button.feed-item').find((button) => button.text().includes('Read item'))

    expect(itemButton).toBeTruthy()
    await itemButton!.trigger('click')

    expect(wrapper.emitted('select')).toEqual([['read-1']])
  })

  it('shows the unreachable state with a retry that emits refresh', async () => {
    const wrapper = mountList(false, "Can't reach GitHub right now.")

    const error = wrapper.get('[data-testid="feed-error"]')
    expect(error.text()).toContain('GitHub unreachable')
    expect(error.text()).toContain("Can't reach GitHub right now.")
    expect(wrapper.text()).not.toContain('Unread item')

    await error.get('button').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('shows the all-caught-up state when the unread filter drains the list', () => {
    const wrapper = mount(FeedList, {
      props: {
        title: 'Unread',
        items: [item('read-1', 'Read item', false)],
        selectedId: null,
        unreadOnly: true,
        countLabel: '1 · 0 unread',
        loadError: null,
      },
    })

    const empty = wrapper.get('[data-testid="feed-empty"]')
    expect(empty.text()).toContain("You're all caught up")
  })

  it('shows the plain empty state without the unread filter', () => {
    const wrapper = mount(FeedList, {
      props: {
        title: 'All items',
        items: [],
        selectedId: null,
        unreadOnly: false,
        countLabel: '0 · 0 unread',
        loadError: null,
      },
    })

    expect(wrapper.get('[data-testid="feed-empty"]').text()).toContain('No items yet')
  })

  it('emits toggle-unread and refresh from header controls', async () => {
    const wrapper = mountList()

    await wrapper.find('button.unread-chip').trigger('click')
    await wrapper.find('button.refresh-chip').trigger('click')

    expect(wrapper.emitted('toggle-unread')).toHaveLength(1)
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })
})
