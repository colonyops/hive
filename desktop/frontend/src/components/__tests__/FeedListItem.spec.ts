import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedListItem from '../FeedListItem.vue'
import type { FeedItem } from '../../types/feed'

const baseItem: FeedItem = {
  id: 'item-1',
  kind: 'PR',
  repo: 'colonyops/hive',
  num: 42,
  title: 'Add desktop shell',
  author: 'hayden',
  age: '5m',
  unread: true,
  labels: ['frontend'],
  branch: 'feat/desktop-ui-shell',
  body: 'Body',
  prompt: 'Prompt',
  url: 'https://github.com/colonyops/hive/pull/42',
}

function mountItem(overrides: Partial<FeedItem> = {}, selected = false) {
  return mount(FeedListItem, {
    props: {
      item: { ...baseItem, ...overrides },
      selected,
    },
  })
}

describe('FeedListItem', () => {
  it('maps pull requests and issues to their kind icon classes', () => {
    const pr = mountItem({ kind: 'PR' })
    expect(pr.find('[data-testid="kind-badge"]').classes()).toContain('kind-icon-pr')
    expect(pr.find('[data-testid="kind-badge"]').attributes('data-kind')).toBe('PR')

    const issue = mountItem({ kind: 'Issue' })
    expect(issue.find('[data-testid="kind-badge"]').classes()).toContain('kind-icon-issue')
    expect(issue.find('[data-testid="kind-badge"]').attributes('data-kind')).toBe('Issue')
  })

  it('shows the unread dot only for unread items', () => {
    const unread = mountItem({ unread: true })
    expect(unread.find('[data-testid="unread-dot"]').exists()).toBe(true)

    const read = mountItem({ unread: false })
    expect(read.find('[data-testid="unread-dot"]').exists()).toBe(false)
  })

  it('renders the notification reason as a chip before the labels, humanized', () => {
    const wrapper = mountItem({ reason: 'review_requested', labels: ['bug'] })
    expect(wrapper.find('[data-testid="reason-chip"]').text()).toBe('review requested')
    expect(wrapper.findAll('span.bg-chip').map((chip) => chip.text())).toEqual(['review requested', 'bug'])
  })

  it('omits the reason chip when the item has no reason', () => {
    const wrapper = mountItem()
    expect(wrapper.find('[data-testid="reason-chip"]').exists()).toBe(false)
    expect(wrapper.findAll('span.bg-chip').map((chip) => chip.text())).toEqual(['frontend'])
  })

  it('applies selected styling', () => {
    const wrapper = mountItem({}, true)
    expect(wrapper.find('button.feed-item').classes()).toContain('selected')
  })

  it('emits select when clicked', async () => {
    const wrapper = mountItem()
    await wrapper.find('button.feed-item').trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
  })
})
