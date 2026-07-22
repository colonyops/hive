import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedListItem from '../FeedListItem.vue'
import type { InboxItem } from '../../types/feed'

const baseItem: InboxItem = {
  id: 42, profileId: 'triage', sourceKind: 'github', sourceScope: 'colonyops/hive', externalId: 'pr-42', title: 'Add desktop shell', url: 'https://github.com/colonyops/hive/pull/42',
  payload: { id: 'pr-42', kind: 'PR', repo: 'colonyops/hive', num: 42, author: 'hayden', branch: 'feat/desktop-ui-shell', body: 'Body' }, revision: 3, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: Date.now(),
}
function mountItem(overrides: Partial<InboxItem> = {}, selected = false) { return mount(FeedListItem, { props: { item: { ...baseItem, ...overrides }, selected } }) }

describe('FeedListItem', () => {
  it('decodes GitHub payload type and metadata through the presentation adapter', () => {
    const wrapper = mountItem()
    expect(wrapper.find('[data-testid="source-badge"]').attributes('data-source')).toBe('github')
    expect(wrapper.find('[data-testid="type-pill"]').classes()).toContain('type-pill-pr')
    expect(wrapper.find('[data-testid="type-pill"]').text()).toBe('Pull Request')
    expect(wrapper.find('[data-testid="item-snippet"]').text()).toContain('hayden — Body')
  })

  it('renders issue styling and an unread indicator only when inbox state is unread', () => {
    const issue = mountItem({ unread: false, payload: { ...baseItem.payload as object, kind: 'Issue' } })
    expect(issue.find('[data-testid="type-pill"]').classes()).toContain('type-pill-issue')
    expect(issue.find('[data-testid="unread-dot"]').exists()).toBe(false)
  })

  it('uses archive reason only in archive presentation and keeps selection styling', () => {
    const wrapper = mount(FeedListItem, { props: { item: { ...baseItem, archivedReason: 'manual' }, view: 'archive', selected: true } })
    expect(wrapper.get('[data-testid="archive-reason"]').text()).toBe('manual')
    expect(wrapper.get('button.feed-item').classes()).toContain('selected')
  })

  it('emits selection intent', async () => {
    const wrapper = mountItem()
    await wrapper.get('button.feed-item').trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
  })
})
