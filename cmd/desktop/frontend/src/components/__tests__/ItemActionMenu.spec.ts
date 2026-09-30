import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ItemActionMenu from '../ItemActionMenu.vue'
import type { ActionView } from '../../types/action'
import type { InboxItem } from '../../types/feed'

const baseItem: InboxItem = {
  id: 42, profileId: 'triage', sourceKind: 'github', sourceScope: 'colonyops/hive', externalId: 'pr-42', title: 'Add desktop shell', url: 'https://github.com/hay-kot/hive-desktop/pull/42',
  payload: { id: 'pr-42', kind: 'PR', repo: 'colonyops/hive', num: 42, author: 'octocat', branch: 'feat/desktop-ui-shell', body: 'Body' }, revision: 3, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: Date.now(),
}
const actions: ActionView[] = [{ id: 'summarize', label: 'Summarize', type: 'launch-session', showInDetail: true, requiresSessionInput: false }]

function mountMenu(overrides: Partial<InboxItem> = {}, provided: ActionView[] | undefined = actions) {
  return mount(ItemActionMenu, { props: { item: { ...baseItem, ...overrides }, actions: provided } })
}

describe('ItemActionMenu', () => {
  it('flips triage labels with item state', () => {
    const unread = mountMenu()
    expect(unread.get('[data-testid="menu-toggle-read"]').text()).toContain('Mark as read')
    expect(unread.get('[data-testid="menu-toggle-archive"]').text()).toContain('Archive')
    expect(unread.get('[data-testid="menu-toggle-ignored"]').text()).toContain('Ignore')

    const disposed = mountMenu({ unread: false, archivedAt: 10, ignoredAt: 20 })
    expect(disposed.get('[data-testid="menu-toggle-read"]').text()).toContain('Mark as unread')
    expect(disposed.get('[data-testid="menu-toggle-archive"]').text()).toContain('Move to inbox')
    expect(disposed.get('[data-testid="menu-toggle-ignored"]').text()).toContain('Stop ignoring')
  })

  it('groups configured actions under a label and emits run-action with the action id', async () => {
    const wrapper = mountMenu()
    expect(wrapper.get('.app-menu-label').text()).toBe('Actions')
    await wrapper.get('[data-testid="menu-action-summarize"]').trigger('click')
    expect(wrapper.emitted('run-action')).toEqual([['summarize']])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('omits the actions group when the item has no configured actions', () => {
    const wrapper = mountMenu({}, [])
    expect(wrapper.find('.app-menu-label').exists()).toBe(false)
  })

  it('hides the link entries when the item has no URL', () => {
    const wrapper = mountMenu({ url: '' })
    expect(wrapper.find('[data-testid="menu-open-browser"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="menu-copy-link"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="menu-copy-contents"]').exists()).toBe(true)
  })

  it('emits a semantic event plus close for every built-in entry', async () => {
    const wrapper = mountMenu()
    await wrapper.get('[data-testid="menu-toggle-read"]').trigger('click')
    await wrapper.get('[data-testid="menu-toggle-archive"]').trigger('click')
    await wrapper.get('[data-testid="menu-open-browser"]').trigger('click')
    await wrapper.get('[data-testid="menu-copy-contents"]').trigger('click')
    expect(wrapper.emitted('set-unread')).toEqual([[false]])
    expect(wrapper.emitted('toggle-archive')).toHaveLength(1)
    expect(wrapper.emitted('open-browser')).toHaveLength(1)
    expect(wrapper.emitted('copy-contents')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(4)
  })
})
