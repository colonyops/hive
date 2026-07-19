import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import NodePalette from '../NodePalette.vue'
import { byType } from '../../registry'
import { NODE_TYPE_MIME } from '../../lib/dragTypes'

describe('NodePalette', () => {
  it('lists every registered node type grouped by category', () => {
    const wrapper = mount(NodePalette)

    const labels = wrapper.findAll('[data-testid="palette-entry-label"]').map((w) => w.text())
    expect(labels.sort()).toEqual(Object.values(byType).map((def) => def.label).sort())

    wrapper.unmount()
  })

  it('filters entries by label as the search query changes', async () => {
    const wrapper = mount(NodePalette)

    await wrapper.get('[data-testid="palette-search"]').setValue('function')

    const labels = wrapper.findAll('[data-testid="palette-entry-label"]').map((w) => w.text())
    expect(labels).toEqual(['Function'])

    wrapper.unmount()
  })

  it('filters entries by the type string, not just the label', async () => {
    const wrapper = mount(NodePalette)

    await wrapper.get('[data-testid="palette-search"]').setValue('github-filter')

    const labels = wrapper.findAll('[data-testid="palette-entry-label"]').map((w) => w.text())
    expect(labels).toEqual(['GitHub filter'])

    wrapper.unmount()
  })

  it('shows an empty state when no node type matches the query', async () => {
    const wrapper = mount(NodePalette)

    await wrapper.get('[data-testid="palette-search"]').setValue('nonexistent-node-type')

    expect(wrapper.find('[data-testid="palette-empty"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="palette-entry"]')).toHaveLength(0)

    wrapper.unmount()
  })

  it('clicking an entry does not add it — dragging onto the canvas is the only way to add a node', async () => {
    const wrapper = mount(NodePalette)

    const entry = wrapper.get('[data-type="feed"]')
    await entry.trigger('click')

    expect(wrapper.emitted('add')).toBeUndefined()

    wrapper.unmount()
  })

  it('each entry is draggable', () => {
    const wrapper = mount(NodePalette)

    const entry = wrapper.get('[data-type="feed"]')
    expect(entry.attributes('draggable')).toBe('true')

    wrapper.unmount()
  })

  it('starting a drag on an entry sets its node type into dataTransfer with a copy effect', async () => {
    const wrapper = mount(NodePalette)
    const setData = vi.fn()
    const dataTransfer = { setData, effectAllowed: '' }

    const entry = wrapper.get('[data-type="function"]')
    await entry.trigger('dragstart', { dataTransfer })

    expect(setData).toHaveBeenCalledWith(NODE_TYPE_MIME, 'function')
    expect(dataTransfer.effectAllowed).toBe('copy')
    expect(wrapper.emitted('add')).toBeUndefined()

    wrapper.unmount()
  })

  it('shows a hover summary (title attribute) drawn from the node type\'s help.md', () => {
    const wrapper = mount(NodePalette)

    const feedEntry = wrapper.get('[data-type="feed"]')
    expect(feedEntry.attributes('title')).toBeTruthy()
    expect(feedEntry.get('[data-testid="palette-entry-summary"]').text()).toBe(feedEntry.attributes('title'))

    wrapper.unmount()
  })
})
