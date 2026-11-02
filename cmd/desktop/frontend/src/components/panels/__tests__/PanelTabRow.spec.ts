import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { Panel } from '../../../lib/panelLayout'
import PanelTabRow from '../PanelTabRow.vue'

const panel: Panel = {
  kind: 'panel',
  id: 'p0',
  tabs: [
    { id: 'a', kind: 'note' },
    { id: 'b', kind: 'note' },
  ],
  activeTab: 'a',
  picking: false,
}

function mountRow(props: Record<string, unknown> = {}) {
  return mount(PanelTabRow, {
    props: {
      panel,
      focused: true,
      label: (tab: { id: string }) => `label ${tab.id}`,
      attention: (tab: { id: string }) => tab.id === 'b',
      dropGap: null,
      dragging: null,
      resizable: false,
      ...props,
    },
    slots: {
      icon: '<i data-testid="icon" />',
      add: '<b data-testid="add" />',
      actions: '<u data-testid="actions" />',
    },
  })
}

describe('PanelTabRow', () => {
  it('labels each tab and marks the shown one selected', () => {
    const wrapper = mountRow()
    expect(wrapper.get('[data-testid="panel-tab-a"] [role="tab"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="panel-tab-b"] [role="tab"]').attributes('aria-selected')).toBe('false')
    expect(wrapper.get('[data-testid="panel-tab-b"] [role="tab"]').attributes('aria-label')).toBe('label b')
    expect(wrapper.get('[data-testid="panel-tab-a"]').attributes('data-shown')).toBe('true')
    expect(wrapper.get('[data-testid="panel-tab-b"]').attributes('data-shown')).toBe('false')
  })

  it('shows no tab as selected while the panel is picking', () => {
    const wrapper = mountRow({ panel: { ...panel, picking: true } })
    expect(wrapper.get('[data-testid="panel-tab-a"]').attributes('data-shown')).toBe('false')
  })

  it('flags a tab that needs attention', () => {
    const wrapper = mountRow()
    expect(wrapper.find('[data-testid="panel-tab-attention-b"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="panel-tab-attention-a"]').exists()).toBe(false)
  })

  it('renders the icon, add and actions slots', () => {
    const wrapper = mountRow()
    expect(wrapper.findAll('[data-testid="icon"]')).toHaveLength(2)
    expect(wrapper.find('[data-testid="add"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="actions"]').exists()).toBe(true)
  })

  it('emits close and select with the tab id', async () => {
    const wrapper = mountRow()
    await wrapper.get('[data-testid="panel-tab-close-b"]').trigger('click')
    await wrapper.get('[data-testid="panel-tab-b"] [role="tab"]').trigger('click')
    expect(wrapper.emitted('close')).toEqual([['b']])
    expect(wrapper.emitted('select')).toEqual([['b']])
  })

  it('draws the drop marker at the target gap', () => {
    expect(mountRow({ dropGap: 1 }).findAll('[data-testid="panel-tab-drop-marker"]')).toHaveLength(1)
    expect(mountRow().find('[data-testid="panel-tab-drop-marker"]').exists()).toBe(false)
  })

  it('marks a drop into a row with no tabs', () => {
    const empty = { ...panel, tabs: [], activeTab: null }
    expect(mountRow({ panel: empty, dropGap: 0 }).find('[data-testid="panel-tab-drop-marker"]').exists()).toBe(true)
    expect(mountRow({ panel: empty }).find('[data-testid="panel-tab-drop-marker"]').exists()).toBe(false)
  })

  it('fades the tab being dragged where it sits', () => {
    const wrapper = mountRow({ dragging: 'b' })
    expect(wrapper.get('[data-testid="panel-tab-b"]').classes()).toContain('opacity-40')
    expect(wrapper.get('[data-testid="panel-tab-a"]').classes()).not.toContain('opacity-40')
  })

  it('starts a boundary resize from the row only when resizable', async () => {
    const plain = mountRow()
    await plain.get('[data-testid="panel-tab-row-p0"]').trigger('pointerdown')
    expect(plain.emitted('resize-start')).toBeUndefined()
    const resizable = mountRow({ resizable: true })
    await resizable.get('[data-testid="panel-tab-row-p0"]').trigger('pointerdown')
    expect(resizable.emitted('resize-start')).toHaveLength(1)
  })

  it('sets a tab in the mono face when asked', () => {
    const wrapper = mountRow({ mono: (tab: { id: string }) => tab.id === 'a' })
    expect(wrapper.get('[data-testid="panel-tab-a"] span.truncate').classes()).toContain('font-mono')
    expect(wrapper.get('[data-testid="panel-tab-b"] span.truncate').classes()).not.toContain('font-mono')
  })
})
