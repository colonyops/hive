import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DetailPane from '../DetailPane.vue'
import type { ActionView } from '../../types/action'
import type { InboxItem } from '../../types/feed'

const item: InboxItem = {
  id: 42, profileId: 'triage', sourceKind: 'github', sourceScope: 'colonyops/hive', externalId: 'pr-42', title: 'Add desktop shell', url: 'https://github.com/colonyops/hive/pull/42',
  payload: { id: 'pr-42', kind: 'PR', repo: 'colonyops/hive', num: 42, author: 'hayden', branch: 'feat/desktop-ui-shell', body: 'Body' }, revision: 1, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: Date.now(),
}
const actions: ActionView[] = [{ id: 'summarize', label: 'Summarize', type: 'launch-session', showInDetail: true, requiresSessionInput: false }]
const payload = (patch: Record<string, unknown>) => ({ ...(item.payload as Record<string, unknown>), ...patch })

describe('DetailPane', () => {
  it('renders source, GitHub context, action cards, and branch metadata from an inbox item', () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })
    expect(wrapper.get('[data-testid="source-badge"]').attributes('data-source')).toBe('github')
    expect(wrapper.text()).toContain('colonyops/hive #42')
    expect(wrapper.findAll('[data-testid="action-card"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="action-footer-branch"]').text()).toBe('feat/desktop-ui-shell')
  })

  it('renders GitHub-flavored markdown and routes body links through open-url', async () => {
    const wrapper = mount(DetailPane, { props: { item: { ...item, payload: payload({ body: '## Steps\n\n- [ ] first\n\n[docs](https://example.com)' }) }, actions } })
    expect(wrapper.get('[data-testid="detail-body"]').find('h2').exists()).toBe(true)
    await wrapper.get('[data-testid="detail-body"] a').trigger('click')
    expect(wrapper.emitted('open-url')).toEqual([['https://example.com']])
  })

  it('formats a current event as now, never now ago', () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })
    expect(wrapper.text()).toContain('· now')
    expect(wrapper.text()).not.toContain('now ago')
  })

  it('emits action and browser intents and has an empty state', async () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })
    await wrapper.get('[data-testid="action-card"]').trigger('click')
    await wrapper.get('button.open-button').trigger('click')
    expect(wrapper.emitted('run-action')).toEqual([['summarize']])
    expect(wrapper.emitted('open-browser')).toHaveLength(1)
    expect(mount(DetailPane, { props: { item: null, actions: [] } }).text()).toContain('Select an item')
  })

  it('persists resize changes', async () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })
    const handle = wrapper.get('[data-testid="resize-handle-detailpane"]')
    await handle.trigger('pointerdown', { clientX: 500, pointerId: 1 })
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 450, pointerId: 1 }))
    await wrapper.vm.$nextTick()
    expect(wrapper.get('aside').element.style.width).toBe('516px')
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 450, pointerId: 1 }))
  })

  it('keeps the type pill non-wrapping and renders each configured action', () => {
    const wrapper = mount(DetailPane, { props: { item, actions: [...actions, { id: 'draft', label: 'Draft reply', type: 'shell', showInDetail: true, requiresSessionInput: false }] } })
    expect(wrapper.get('.kind-pill').classes()).toEqual(expect.arrayContaining(['shrink-0', 'whitespace-nowrap']))
    expect(wrapper.findAll('[data-testid="action-card"]')).toHaveLength(2)
  })

  it('does not render an empty body container and keeps long branch metadata in its label/value stack', () => {
    const wrapper = mount(DetailPane, { props: { item: { ...item, payload: payload({ body: '', branch: 'feat/a-very-long-branch-name-that-must-wrap' }) }, actions } })
    expect(wrapper.find('[data-testid="detail-body"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="action-footer-meta"]').text()).toContain('Runs headless (batch) on')
    expect(wrapper.get('[data-testid="action-footer-branch"]').text()).toContain('a-very-long-branch')
  })

  it('offers read, archive, and ignore actions from the item menu', async () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })
    await wrapper.get('[data-testid="item-actions-toggle"]').trigger('click')
    const entries = wrapper.get('[data-testid="item-actions-menu"]').findAll('button')
    expect(entries.map((entry) => entry.text())).toEqual(['Mark as read', 'Archive', 'Ignore'])
    await entries[2]!.trigger('click')
    expect(wrapper.emitted('toggle-ignored')).toHaveLength(1)
  })

  it('renders the Observed activity timeline in supplied chronological order', () => {
    const wrapper = mount(DetailPane, { props: { item, actions, events: [
      { id: 1, itemId: 42, kind: 'created', transition: 'created', attention: 'activity', summary: 'first observation', createdAt: 1 },
      { id: 2, itemId: 42, kind: 'updated', transition: 'updated', attention: 'activity', summary: 'second observation', createdAt: 2 },
    ] } })
    const timeline = wrapper.get('[data-testid="observed-activity"]')
    expect(timeline.text()).toContain('OBSERVED ACTIVITY')
    expect(timeline.findAll('li')).toHaveLength(2)
    expect(timeline.findAll('li')[0]!.text()).toContain('first observation')
    expect(timeline.findAll('li')[1]!.text()).toContain('second observation')
  })

})
