import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedListItem from '../FeedListItem.vue'
import type { InboxItem } from '../../types/feed'

const baseItem: InboxItem = {
  id: 42, profileId: 'triage', sourceKind: 'github', sourceScope: 'colonyops/hive', externalId: 'pr-42', title: 'Add desktop shell', url: 'https://github.com/hay-kot/hive-desktop/pull/42',
  payload: { id: 'pr-42', kind: 'PR', repo: 'colonyops/hive', num: 42, author: 'octocat', branch: 'feat/desktop-ui-shell', body: 'Body', ci: 'passing', review: 'approved', additions: 42, deletions: 7 }, revision: 3, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: Date.now(),
}
function mountItem(overrides: Partial<InboxItem> = {}, selected = false) { return mount(FeedListItem, { props: { item: { ...baseItem, ...overrides }, selected } }) }

describe('FeedListItem', () => {
  it('decodes GitHub payload type and metadata through the presentation adapter', () => {
    const wrapper = mountItem()
    expect(wrapper.find('[data-testid="source-badge"]').attributes('data-source')).toBe('github')
    expect(wrapper.find('[data-testid="type-pill"]').classes()).toContain('type-pill-pr')
    expect(wrapper.find('[data-testid="type-pill"]').text()).toBe('Pull Request')
    expect(wrapper.get('[data-testid="item-byline"]').text()).toContain('octocat')
    expect(wrapper.get('[data-testid="item-byline"]').text()).toContain('Approved')
    expect(wrapper.findAll('[data-testid="metadata-separator"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="item-byline"]').text()).not.toContain('Body')
    expect(wrapper.get('[data-testid="item-byline"] [data-testid="pr-ci"]').attributes('aria-label')).toBe('Checks pass')
    expect(wrapper.get('[data-testid="pr-ci"]').classes()).toContain('text-severity-success/80')
    expect(wrapper.get('[data-testid="pr-review"]').text()).toBe('Approved')
    expect(wrapper.get('[data-testid="pr-review"]').classes()).toContain('text-severity-success/80')
    expect(wrapper.get('[data-testid="pr-lines"] span:first-child').classes()).toContain('text-severity-success/70')
    expect(wrapper.get('[data-testid="pr-lines"] span:last-child').classes()).toContain('text-severity-error/70')
    expect(wrapper.get('[data-testid="pr-lines"]').text()).toContain('+42')
    expect(wrapper.get('[data-testid="pr-lines"]').text()).toContain('−7')
  })

  it('renders issue styling and marks unread state through title weight', () => {
    expect(mountItem().get('[data-testid="item-title"]').classes()).toContain('font-semibold')

    const issue = mountItem({ unread: false, payload: { ...baseItem.payload as object, kind: 'Issue' } })
    expect(issue.find('[data-testid="type-pill"]').classes()).toContain('type-pill-issue')
    expect(issue.get('[data-testid="item-title"]').classes()).toContain('font-normal')
    expect(issue.get('[data-testid="item-byline"]').text()).toBe('octocat')
    expect(issue.find('[data-testid="pr-metadata"]').exists()).toBe(false)
  })

  it('uses compact colored glyphs for pull request status', () => {
    const failing = mountItem({ payload: { ...baseItem.payload as object, ci: 'failing', review: 'open' } })
    expect(failing.get('[data-testid="pr-ci"]').classes()).toContain('text-severity-error/80')
    expect(failing.find('[data-testid="metadata-separator"]').exists()).toBe(false)

    const pending = mountItem({ payload: { ...baseItem.payload as object, ci: 'pending' } })
    expect(pending.get('[data-testid="pr-ci"]').classes()).toContain('text-severity-warning/80')
    expect(pending.get('[data-testid="pr-ci"] svg').classes()).toContain('motion-safe:animate-spin')
  })

  it('shows only approved and draft review states beside the author', () => {
    const draft = mountItem({ payload: { ...baseItem.payload as object, review: 'draft' } })
    expect(draft.get('[data-testid="pr-review"]').text()).toBe('Draft')

    for (const review of ['open', 'review_required', 'changes_requested']) {
      const wrapper = mountItem({ payload: { ...baseItem.payload as object, review } })
      expect(wrapper.find('[data-testid="pr-review"]').exists()).toBe(false)
    }
  })

  it('omits metadata for notification PRs and hides the no-CI state', () => {
    const notification = mountItem({ payload: { id: 'pr-42', kind: 'PR', repo: 'colonyops/hive', num: 42 } })
    expect(notification.find('[data-testid="pr-metadata"]').exists()).toBe(false)

    const noChecks = mountItem({ payload: { ...baseItem.payload as object, ci: 'none', review: 'open', additions: 0, deletions: 0 } })
    expect(noChecks.find('[data-testid="pr-ci"]').exists()).toBe(false)
    expect(noChecks.find('[data-testid="pr-review"]').exists()).toBe(false)
    expect(noChecks.get('[data-testid="pr-lines"]').text()).toContain('+0')
    expect(noChecks.get('[data-testid="pr-lines"]').text()).toContain('−0')
  })

  it('uses archive reason only in archived presentation and keeps selection styling', () => {
    const wrapper = mount(FeedListItem, { props: { item: { ...baseItem, archivedReason: 'manual' }, archived: true, selected: true } })
    expect(wrapper.get('[data-testid="archive-reason"]').text()).toBe('manual')
    expect(wrapper.get('[data-testid="feed-item"]').classes()).toContain('selected')
  })

  it('selects on a click, and activates on the gestures that mean "open this"', async () => {
    const wrapper = mountItem()
    await wrapper.get('[data-testid="feed-item"]').trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
    expect(wrapper.emitted('activate')).toBeUndefined()

    await wrapper.get('[data-testid="feed-item"]').trigger('dblclick')
    await wrapper.get('[data-testid="feed-item"]').trigger('keydown.enter')
    await wrapper.get('[data-testid="feed-item"]').trigger('keydown.space')
    expect(wrapper.emitted('activate')).toHaveLength(3)
    expect(wrapper.emitted('select')).toHaveLength(1)
  })

  it('uses an accessible checkbox and toggles instead of opening while selecting', async () => {
    const wrapper = mount(FeedListItem, { props: { item: baseItem, selected: false, selectionMode: true, checked: true } })
    const row = wrapper.get('[data-testid="feed-item"]')
    expect(row.attributes('role')).toBe('checkbox')
    expect(row.attributes('aria-label')).toBe('Select Add desktop shell')
    expect(row.attributes('aria-checked')).toBe('true')
    expect(wrapper.get('[data-testid="feed-item-checkbox"]').attributes('aria-hidden')).toBe('true')

    await row.trigger('click')
    await row.trigger('keydown', { key: ' ' })
    expect(wrapper.emitted('toggle-selection')).toHaveLength(2)
    expect(wrapper.emitted('select')).toBeUndefined()
    expect(wrapper.find('[data-testid="row-menu-toggle"]').exists()).toBe(false)
  })

  it('offers archive and open-in-browser from the hover pill without selecting the row', async () => {
    const wrapper = mountItem()
    await wrapper.get('[data-testid="row-archive"]').trigger('click')
    await wrapper.get('[data-testid="row-open"]').trigger('click')
    expect(wrapper.emitted('toggle-archive')).toHaveLength(1)
    expect(wrapper.emitted('open-browser')).toHaveLength(1)
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('swaps the archive slot for stop-ignoring in trash presentation', () => {
    const wrapper = mount(FeedListItem, { props: { item: { ...baseItem, ignoredAt: 5 }, trash: true, selected: false } })
    expect(wrapper.find('[data-testid="row-archive"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="row-restore"]').attributes('aria-label')).toBe('Stop ignoring')
  })

  it('renders webhook items with the source node icon and no open affordance without a URL', () => {
    const wrapper = mount(FeedListItem, { props: {
      item: { ...baseItem, sourceKind: 'webhook', sourceScope: 'sources.webhook-1', url: '', payload: { id: 'run-1', status: 'failure' } },
      selected: false,
      sourceIcons: { 'sources.webhook-1': 'bell' },
    } })
    const badge = wrapper.get('[data-testid="source-badge"]')
    expect(badge.attributes('data-source')).toBe('webhook')
    expect(badge.find('svg').exists()).toBe(true)
    expect(wrapper.get('[data-testid="type-pill"]').text()).toBe('Item')
    expect(wrapper.find('[data-testid="row-open"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('#')
  })

  it('falls back to the webhook glyph when the source node has no configured icon', () => {
    const wrapper = mount(FeedListItem, { props: {
      item: { ...baseItem, sourceKind: 'webhook', sourceScope: 'sources.webhook-1', url: '', payload: {} },
      selected: false,
    } })
    expect(wrapper.get('[data-testid="source-badge"]').find('svg').exists()).toBe(true)
  })

  it('opens the item menu from the kebab and from right-click and relays its intents', async () => {
    const wrapper = mountItem()
    await wrapper.get('[data-testid="row-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="menu-copy-link"]').trigger('click')
    expect(wrapper.emitted('copy-link')).toHaveLength(1)
    expect(wrapper.find('[data-testid="row-menu"]').exists()).toBe(false)

    await wrapper.get('[data-testid="feed-item"]').trigger('contextmenu')
    expect(wrapper.find('[data-testid="row-menu"]').exists()).toBe(true)
    await wrapper.get('[data-testid="menu-toggle-read"]').trigger('click')
    expect(wrapper.emitted('set-unread')).toEqual([[false]])
    expect(wrapper.emitted('select')).toBeUndefined()
  })
})
