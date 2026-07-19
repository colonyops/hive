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

function mountCanvas(props: {
  flow?: EditorFlow
  layout?: WireLayout
  latestRunByNode?: Map<string, NodeRunRecord>
  nodeRuns?: NodeRunRecord[]
  runningNodeIds?: Set<string>
  focusNodeId?: string | null
} = {}) {
  return mount(FlowsCanvas, {
    props: {
      flow: props.flow ?? flow(),
      layout: props.layout ?? layout(),
      latestRunByNode: props.latestRunByNode ?? new Map(),
      nodeRuns: props.nodeRuns ?? [],
      runningNodeIds: props.runningNodeIds,
      focusNodeId: props.focusNodeId,
    },
  })
}

function run(overrides: Partial<NodeRunRecord> = {}): NodeRunRecord {
  return { flowId: 'flow-1', nodeId: 'source', ok: true, inCount: 3, outCount: 3, dropCount: 0, err: '', durMs: 5, endedAt: Date.now() * 1e6, ...overrides }
}

async function clickNode(wrapper: ReturnType<typeof mountCanvas>, testid: string) {
  const card = wrapper.get(`[data-testid="${testid}"]`)
  await card.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 })
  window.dispatchEvent(new PointerEvent('pointerup', { clientX: 100, clientY: 100 }))
  await nextTick()
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

  it('renders the 176×52 card geometry (8a/8c) with 9×13 ports', () => {
    const wrapper = mountCanvas()

    // Outer wrapper carries the 176px card width (and its layout translate).
    expect(wrapper.get('[data-testid="flow-node-source"]').attributes('style')).toContain('width: 176px')
    // Inner card is 52px tall, 2px radius, with a 6px (w-1.5 = 0.375rem = 6px) left role cap.
    const inner = wrapper.get('[data-testid="flow-node-source"] > div')
    expect(inner.classes()).toContain('h-[52px]')
    expect(inner.classes()).toContain('rounded-[2px]')
    // Ports render as 9×13 rounded rects.
    const outPort = wrapper.get('[data-testid="port-out-source-0"]')
    expect(outPort.attributes('style')).toContain('width: 9px')
    expect(outPort.attributes('style')).toContain('height: 13px')

    wrapper.unmount()
  })

  it('shows ok status with in/out counts and error status with the run error, done below the card', () => {
    const runs = new Map<string, NodeRunRecord>([
      ['source', run({ nodeId: 'source', ok: true, inCount: 4, outCount: 4 })],
      ['filter', run({ nodeId: 'filter', ok: false, err: 'boom' })],
    ])
    const wrapper = mountCanvas({ latestRunByNode: runs })

    expect(wrapper.get('[data-testid="flow-node-source"] [data-testid="flow-node-status"]').text()).toContain('4 → 4')
    expect(wrapper.get('[data-testid="flow-node-filter"] [data-testid="flow-node-status"]').text()).toBe('error: boom')
    // The status line renders as a sibling below the 52px card, not inside it.
    const wrapperEl = wrapper.get('[data-testid="flow-node-source"]').element
    const cardEl = wrapperEl.children[0] as HTMLElement
    expect(cardEl.querySelector('[data-testid="flow-node-status"]')).toBeNull()
    expect(wrapperEl.querySelector('[data-testid="flow-node-status"]')).not.toBeNull()

    wrapper.unmount()
  })

  it('shows the running state (blue, pulsing) for a node in runningNodeIds, overriding its latest run', () => {
    const runs = new Map<string, NodeRunRecord>([['source', run({ nodeId: 'source', ok: true })]])
    const wrapper = mountCanvas({ latestRunByNode: runs, runningNodeIds: new Set(['source']) })

    const status = wrapper.get('[data-testid="flow-node-source"] [data-testid="flow-node-status"]')
    expect(status.text()).toBe('running…')

    const dot = wrapper.get('[data-testid="flow-node-source"] .rounded-full')
    expect(dot.classes()).toContain('hive-pulse')
    expect(dot.attributes('style')).toContain('var(--color-severity-running)')

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

  it('clicking a node (no drag) opens the NodeInspector for that node, not the drawer', async () => {
    const wrapper = mountCanvas()

    await clickNode(wrapper, 'flow-node-filter')

    expect(wrapper.get('[data-testid="node-inspector-title"]').text()).toContain('GitHub filter')
    expect(document.querySelector('[data-testid="node-editor"]')).toBeNull()

    wrapper.unmount()
  })

  it('clicking empty canvas space deselects the node and closes the inspector', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-filter')
    expect(wrapper.find('[data-testid="node-inspector-panel"]').exists()).toBe(true)

    await wrapper.get('[data-testid="flows-canvas"]').trigger('click')

    expect(wrapper.find('[data-testid="node-inspector-panel"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('the inspector\'s RECENT list reads from nodeRuns (not just the latest run)', async () => {
    const runs = [
      run({ nodeId: 'filter', ok: true, inCount: 5, outCount: 3, endedAt: Date.now() * 1e6 }),
      run({ nodeId: 'filter', ok: false, err: 'timeout', endedAt: Date.now() * 1e6 }),
      run({ nodeId: 'source', ok: true }), // a different node — must not appear
    ]
    const wrapper = mountCanvas({ nodeRuns: runs })

    await clickNode(wrapper, 'flow-node-filter')

    const rows = wrapper.findAll('[data-testid="node-inspector-recent-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('5 → 3')
    expect(rows[1]!.text()).toContain('timeout')

    wrapper.unmount()
  })

  it('the inspector\'s Edit button opens the NodeEditorDrawer for the selected node and hides the inspector', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-filter')

    await wrapper.get('[data-testid="node-inspector-edit"]').trigger('click')

    expect(document.querySelector('[data-testid="node-editor-title"]')?.textContent).toBe('GitHub filter')
    expect(wrapper.find('[data-testid="node-inspector-panel"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('dragging a node past the threshold emits move and does not open the inspector', async () => {
    const wrapper = mountCanvas()
    const card = wrapper.get('[data-testid="flow-node-source"]') // layout position {x:10, y:20}

    await card.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 })
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 130, clientY: 100 }))
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 130, clientY: 100 }))
    await nextTick()

    expect(wrapper.emitted('move')).toEqual([['source', 40, 20]])
    expect(wrapper.find('[data-testid="node-inspector-panel"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('a drawer save re-emits update-node, closes the drawer, and keeps the node selected', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-feed')
    await wrapper.get('[data-testid="node-inspector-edit"]').trigger('click')

    document.querySelector<HTMLButtonElement>('[data-testid="node-editor-save"]')!.click()
    await nextTick()

    expect(wrapper.emitted('update-node')).toEqual([[{ id: 'feed', type: 'feed', disabled: false, config: { feed: 'inbox' } }]])
    expect(document.querySelector('[data-testid="node-editor"]')).toBeNull()
    expect(wrapper.find('[data-testid="node-inspector-panel"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('a drawer delete re-emits delete-node and closes both the drawer and the inspector', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-feed')
    await wrapper.get('[data-testid="node-inspector-edit"]').trigger('click')

    document.querySelector<HTMLButtonElement>('[data-testid="node-editor-delete"]')!.click()
    await nextTick()
    document.querySelector<HTMLButtonElement>('[data-testid="node-editor-delete-confirm"]')!.click()
    await nextTick()

    expect(wrapper.emitted('delete-node')).toEqual([['feed']])
    expect(document.querySelector('[data-testid="node-editor"]')).toBeNull()
    expect(wrapper.find('[data-testid="node-inspector-panel"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('zoomIn/zoomOut adjust the exposed zoom level for the toolbar to display', () => {
    const wrapper = mountCanvas()

    expect(wrapper.vm.zoom).toBe(1)

    wrapper.vm.zoomIn()
    expect(wrapper.vm.zoom).toBe(1.1)

    wrapper.vm.zoomOut()
    wrapper.vm.zoomOut()
    expect(wrapper.vm.zoom).toBe(0.9)

    wrapper.unmount()
  })

  it('fit() scales content to fit the (fallback, non-measured) viewport', () => {
    const wrapper = mountCanvas()

    wrapper.vm.fit()

    // Content bbox with 176×52 cards: x in [10, 776] (filter card right edge
    // 400+176, feed fallback-positioned at grid index 2 -> x=600, right edge
    // 776), y in [20, 132] (feed fallback y=80, +52 card height). happy-dom
    // reports clientWidth/clientHeight as 0, so fit() falls back to
    // 1200x800 with 48px padding: scaleX=(1200-96)/766≈1.441,
    // scaleY=(800-96)/112≈6.286 — the smaller (scaleX) wins, clamped into
    // [0.25, 1.5].
    expect(Math.round(wrapper.vm.zoom * 100)).toBe(144)

    wrapper.unmount()
  })

  it('focusNodeId selects the node and center-pans on it (reusing fit()\'s bbox/scale/pan mechanism)', async () => {
    const wrapper = mountCanvas()
    expect(wrapper.find('[data-testid="node-inspector-panel"]').exists()).toBe(false)

    await wrapper.setProps({ focusNodeId: 'filter' })

    expect(wrapper.get('[data-testid="node-inspector-title"]').text()).toContain('GitHub filter')
    // A single 176×52 node's bbox is tiny next to the (fallback) 1200×800
    // viewport, so fitToBBox clamps zoom to its 1.5 max.
    expect(wrapper.vm.zoom).toBe(1.5)

    wrapper.unmount()
  })
})
