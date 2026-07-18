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
