import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FeedItemsPreview, { type FeedItemsClient } from '../FeedItemsPreview.vue'
import type { InboxItem } from '../../../types/feed'

function item(overrides: Partial<InboxItem> = {}): InboxItem {
  return { id: 1, profileId: 'flow', sourceKind: 'github', sourceScope: 'acme/app', externalId: 'item-1', title: 'Fix the thing', url: '', payload: { kind: 'PR', repo: 'acme/app', author: 'octocat' }, revision: 1, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: 1, ...overrides }
}

describe('FeedItemsPreview', () => {
  it('does not load until a feed node is selected', () => {
    const client: FeedItemsClient = { feedItems: vi.fn() }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: null, client } })
    expect(wrapper.find('[data-testid="feed-preview-empty"]').exists()).toBe(true)
    expect(client.feedItems).not.toHaveBeenCalled()
  })

  it('loads inbox items claimed by the selected feed and renders title/repository/author', async () => {
    const client: FeedItemsClient = { feedItems: vi.fn().mockResolvedValue([item(), item({ id: 2, title: 'Second', unread: false, payload: { kind: 'Issue', repo: 'acme/other', author: 'bob' } })]) }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: 'flow/feed', client } })
    await flushPromises()
    expect(client.feedItems).toHaveBeenCalledWith('flow/feed')
    expect(wrapper.findAll('[data-testid^="feed-preview-item-"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('Fix the thing')
    expect(wrapper.text()).toContain('acme/other · bob')
  })

  it('reloads on feed switch and presents empty and error results inline', async () => {
    const client: FeedItemsClient = { feedItems: vi.fn().mockResolvedValue([]) }
    const wrapper = mount(FeedItemsPreview, { props: { feedId: 'flow/one', client } })
    await flushPromises()
    expect(wrapper.find('[data-testid="feed-preview-no-items"]').exists()).toBe(true)
    await wrapper.setProps({ feedId: 'flow/two' }); await flushPromises()
    expect(client.feedItems).toHaveBeenLastCalledWith('flow/two')
    ;(client.feedItems as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('boom'))
    await wrapper.setProps({ feedId: 'flow/three' }); await flushPromises()
    expect(wrapper.get('[data-testid="feed-preview-error"]').text()).toBe('boom')
  })
})
