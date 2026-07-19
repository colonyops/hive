import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DetailPane from '../DetailPane.vue'
import type { Action, FeedItem } from '../../types/feed'

const item: FeedItem = {
  id: 'item-1',
  kind: 'PR',
  repo: 'colonyops/hive',
  num: 42,
  title: 'Add desktop shell',
  author: 'hayden',
  age: '5m',
  unread: true,
  feedId: 'triage/desktop',
  labels: [],
  branch: 'feat/desktop-ui-shell',
  body: 'Body',
  prompt: 'Prompt',
  url: 'https://github.com/colonyops/hive/pull/42',
}

const actions: Action[] = [
  {
    id: 'summarize',
    icon: 'sparkles',
    color: '#f59e0b',
    title: 'Summarize thread',
    sub: 'Generate a concise summary',
    primary: true,
  },
  {
    id: 'draft',
    icon: 'list',
    color: '#38bdf8',
    title: 'Draft reply',
    sub: 'Write a response',
    primary: false,
  },
]

describe('DetailPane', () => {
  it('renders one action card per action', () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })

    expect(wrapper.findAll('[data-testid="action-card"]')).toHaveLength(2)
  })

  it('emits run-action with the action id', async () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })
    const draftButton = wrapper.findAll('button.action-card').find((button) => button.text().includes('Draft reply'))

    expect(draftButton).toBeTruthy()
    await draftButton!.trigger('click')

    expect(wrapper.emitted('run-action')).toEqual([['draft']])
  })

  it('renders the empty state when no item is selected', () => {
    const wrapper = mount(DetailPane, { props: { item: null, actions: [] } })

    expect(wrapper.text()).toContain('Select an item to inspect')
  })

  it('renders a resize handle that widens the panel on drag and persists the width', async () => {
    const wrapper = mount(DetailPane, { props: { item, actions } })
    const aside = wrapper.get('aside').element as HTMLElement
    expect(aside.style.width).toBe('466px') // default

    const handle = wrapper.get('[data-testid="resize-handle-detailpane"]')
    expect(handle.attributes('role')).toBe('separator')

    await handle.trigger('pointerdown', { clientX: 500, pointerId: 1 })
    // edge is 'left': dragging left (toward the FeedList) grows the pane.
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 450, pointerId: 1 }))
    await wrapper.vm.$nextTick()

    expect(aside.style.width).toBe('516px')

    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 450, pointerId: 1 }))
    expect(localStorage.getItem('hive.panel.detailpane')).toBe('516')
  })
})
