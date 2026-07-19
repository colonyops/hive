import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FeedItemsPreview, { type FeedItemsClient } from '../FeedItemsPreview.vue'
import type { FeedItem } from '../../types'

function item(overrides: Partial<FeedItem> = {}): FeedItem {
  return {
    feedId: 'inbox',
    itemId: 'item-1',
    payload: { title: 'Fix the thing', repo: 'acme/app', author: 'octocat' },
    updatedAt: 0,
    unread: true,
    ...overrides,
  }
}

describe('FeedItemsPreview', () => {
  it('shows an empty state when no feed node is selected', () => {
    const client: FeedItemsClient = { feedItems: vi.fn() }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: null, client } })

    expect(wrapper.find('[data-testid="feed-preview-empty"]').exists()).toBe(true)
    expect(client.feedItems).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it("loads and renders a feed's items (title/repo/author) from the injected client", async () => {
    const items = [
      item({ itemId: 'a', payload: { title: 'PR one', repo: 'acme/app', author: 'alice' } }),
      item({ itemId: 'b', payload: { title: 'PR two', repo: 'acme/other', author: 'bob' }, unread: false }),
    ]
    const client: FeedItemsClient = { feedItems: vi.fn().mockResolvedValue(items) }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: 'inbox', client } })
    await flushPromises()

    expect(client.feedItems).toHaveBeenCalledWith('inbox')
    const rows = wrapper.findAll('[data-testid^="feed-preview-item-"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('PR one')
    expect(rows[0]!.text()).toContain('acme/app')
    expect(rows[0]!.text()).toContain('alice')
    expect(rows[1]!.text()).toContain('PR two')

    wrapper.unmount()
  })

  it('re-loads when feedId changes', async () => {
    const client: FeedItemsClient = { feedItems: vi.fn().mockResolvedValue([item()]) }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: 'inbox', client } })
    await flushPromises()
    ;(client.feedItems as ReturnType<typeof vi.fn>).mockClear()

    await wrapper.setProps({ feedId: 'other-feed' })
    await flushPromises()

    expect(client.feedItems).toHaveBeenCalledWith('other-feed')

    wrapper.unmount()
  })

  it('shows a no-items state when the feed is empty', async () => {
    const client: FeedItemsClient = { feedItems: vi.fn().mockResolvedValue([]) }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: 'inbox', client } })
    await flushPromises()

    expect(wrapper.find('[data-testid="feed-preview-no-items"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('surfaces a load failure inline', async () => {
    const client: FeedItemsClient = { feedItems: vi.fn().mockRejectedValue(new Error('boom')) }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: 'inbox', client } })
    await flushPromises()

    expect(wrapper.get('[data-testid="feed-preview-error"]').text()).toBe('boom')

    wrapper.unmount()
  })
})
