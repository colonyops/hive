import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import FlowsCanvas from '../FlowsCanvas.vue'
import type { EditorFlow, NodeRunRecord, WireLayout } from '../../lib/wireFlow'

function flow(overrides: Partial<EditorFlow> = {}): EditorFlow {
  return {
    id: 'flow-1',
    name: 'My flow',
    enabled: true,
    nodes: [
      { id: 'source', type: 'github-source', config: { source: 'my-prs' } },
      { id: 'filter', type: 'github-filter', config: {} },
      { id: 'feed', type: 'feed', config: { feed: 'inbox' } },
    ],
    wires: [
      { from: 'source', to: 'filter' },
      { from: 'filter', out: 0, to: 'feed' },
    ],
    ...overrides,
  }
}

function layout(overrides: Partial<WireLayout['nodes']> = {}): WireLayout {
  return { nodes: { source: { x: 10, y: 20 }, filter: { x: 400, y: 20 }, ...overrides } }
}

function mountCanvas(props: { flow?: EditorFlow; layout?: WireLayout; latestRunByNode?: Map<string, NodeRunRecord> } = {}) {
  return mount(FlowsCanvas, {
    props: {
      flow: props.flow ?? flow(),
      layout: props.layout ?? layout(),
      latestRunByNode: props.latestRunByNode ?? new Map(),
    },
  })
}

function run(overrides: Partial<NodeRunRecord> = {}): NodeRunRecord {
  return { flowId: 'flow-1', nodeId: 'source', ok: true, inCount: 3, outCount: 3, dropCount: 0, err: '', durMs: 5, endedAt: Date.now() * 1e6, ...overrides }
}

describe('FlowsCanvas', () => {
  it('renders one card per node with its title, type, and idle status by default', () => {
    const wrapper = mountCanvas()

    expect(wrapper.get('[data-testid="flow-node-source"] [data-testid="flow-node-title"]').text()).toBe('GitHub source')
    expect(wrapper.get('[data-testid="flow-node-filter"] [data-testid="flow-node-title"]').text()).toBe('GitHub filter')
    expect(wrapper.get('[data-testid="flow-node-feed"] [data-testid="flow-node-title"]').text()).toBe('Feed')
    expect(wrapper.get('[data-testid="flow-node-source"] [data-testid="flow-node-status"]').text()).toBe('idle')

    wrapper.unmount()
  })

  it('prefers a node\'s own name over its type label', () => {
    const wrapper = mountCanvas({ flow: flow({ nodes: [{ id: 'source', type: 'github-source', name: 'My PRs', config: { source: 'my-prs' } }] }) })

    expect(wrapper.get('[data-testid="flow-node-source"] [data-testid="flow-node-title"]').text()).toBe('My PRs')

    wrapper.unmount()
  })

  it('shows ok status with in/out counts and error status with the run error', () => {
    const runs = new Map<string, NodeRunRecord>([
      ['source', run({ nodeId: 'source', ok: true, inCount: 4, outCount: 4 })],
      ['filter', run({ nodeId: 'filter', ok: false, err: 'boom' })],
    ])
    const wrapper = mountCanvas({ latestRunByNode: runs })

    expect(wrapper.get('[data-testid="flow-node-source"] [data-testid="flow-node-status"]').text()).toContain('4→4')
    expect(wrapper.get('[data-testid="flow-node-filter"] [data-testid="flow-node-status"]').text()).toBe('boom')

    wrapper.unmount()
  })

  it('renders one wire path per flow wire', () => {
    const wrapper = mountCanvas()

    expect(wrapper.findAll('[data-testid="flow-wire"]')).toHaveLength(2)

    wrapper.unmount()
  })

  it('falls back to a deterministic grid position for a node missing from the layout', () => {
    const wrapper = mountCanvas() // layout() only positions source/filter — feed (index 2) falls back

    const style = wrapper.get('[data-testid="flow-node-feed"]').attributes('style') ?? ''
    expect(style).toContain('translate(600px, 80px)')

    wrapper.unmount()
  })

  it('shows an empty-state message when the flow has no nodes', () => {
    const wrapper = mountCanvas({ flow: flow({ nodes: [], wires: [] }) })

    expect(wrapper.find('[data-testid="canvas-empty"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('clicking a node (no drag) opens the NodeEditorDrawer for that node', async () => {
    const wrapper = mountCanvas()
    const card = wrapper.get('[data-testid="flow-node-filter"]')

    await card.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 })
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 100, clientY: 100 }))
    await nextTick()

    expect(document.querySelector('[data-testid="node-editor-title"]')?.textContent).toBe('GitHub filter')

    wrapper.unmount()
  })

  it('dragging a node past the threshold emits move and does not open the drawer', async () => {
    const wrapper = mountCanvas()
    const card = wrapper.get('[data-testid="flow-node-source"]') // layout position {x:10, y:20}

    await card.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 })
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 130, clientY: 100 }))
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 130, clientY: 100 }))
    await nextTick()

    expect(wrapper.emitted('move')).toEqual([['source', 40, 20]])
    expect(document.querySelector('[data-testid="node-editor"]')).toBeNull()

    wrapper.unmount()
  })

  it('a drawer save re-emits update-node and closes the drawer; delete re-emits delete-node', async () => {
    const wrapper = mountCanvas()
    const card = wrapper.get('[data-testid="flow-node-feed"]')
    await card.trigger('pointerdown', { button: 0, clientX: 0, clientY: 0 })
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 0, clientY: 0 }))
    await nextTick()

    document.querySelector<HTMLButtonElement>('[data-testid="node-editor-save"]')!.click()
    await nextTick()

    expect(wrapper.emitted('update-node')).toEqual([[{ id: 'feed', type: 'feed', disabled: false, config: { feed: 'inbox' } }]])
    expect(document.querySelector('[data-testid="node-editor"]')).toBeNull()

    wrapper.unmount()
  })

  it('zoom in/out buttons adjust the displayed zoom level', async () => {
    const wrapper = mountCanvas()

    expect(wrapper.get('[data-testid="canvas-zoom-level"]').text()).toBe('100%')

    await wrapper.get('[data-testid="canvas-zoom-in"]').trigger('click')
    expect(wrapper.get('[data-testid="canvas-zoom-level"]').text()).toBe('110%')

    await wrapper.get('[data-testid="canvas-zoom-out"]').trigger('click')
    await wrapper.get('[data-testid="canvas-zoom-out"]').trigger('click')
    expect(wrapper.get('[data-testid="canvas-zoom-level"]').text()).toBe('90%')

    wrapper.unmount()
  })

  it('Fit scales content to fit the (fallback, non-measured) viewport', async () => {
    const wrapper = mountCanvas()

    await wrapper.get('[data-testid="canvas-fit"]').trigger('click')

    // Content bbox: x in [10, 808] (filter card right edge 400+208, feed
    // fallback-positioned at grid index 2 -> x=600, right edge 808), y in
    // [20, 144] (feed fallback y=80, +64 card height). happy-dom reports
    // clientWidth/clientHeight as 0, so fit() falls back to 1200x800 with
    // 48px padding: scaleX=(1200-96)/798≈1.383, scaleY=(800-96)/124≈5.677 —
    // the smaller (scaleX) wins, clamped into [0.25, 1.5].
    expect(wrapper.get('[data-testid="canvas-zoom-level"]').text()).toBe('138%')

    wrapper.unmount()
  })
})
