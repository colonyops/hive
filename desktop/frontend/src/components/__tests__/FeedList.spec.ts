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
    labels: null,
    branch: 'feat/desktop-ui-shell',
    body: 'Body',
    prompt: 'Prompt',
    url: 'https://github.com/colonyops/hive/pull/42',
  }
}

const items = [item('unread-1', 'Unread item', true), item('read-1', 'Read item', false)]

function mountList(unreadOnly = false) {
  return mount(FeedList, {
    props: {
      title: 'All items',
      items,
      selectedId: null,
      unreadOnly,
      countLabel: '2 · 1 unread',
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

  it('emits toggle-unread and refresh from header controls', async () => {
    const wrapper = mountList()

    await wrapper.find('button.unread-chip').trigger('click')
    await wrapper.find('button.refresh-chip').trigger('click')

    expect(wrapper.emitted('toggle-unread')).toHaveLength(1)
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })
})
