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
  runningNodeIds?: Set<string>
  focusNodeId?: string | null
} = {}) {
  return mount(FlowsCanvas, {
    props: {
      flow: props.flow ?? flow(),
      layout: props.layout ?? layout(),
      latestRunByNode: props.latestRunByNode ?? new Map(),
      runningNodeIds: props.runningNodeIds,
      focusNodeId: props.focusNodeId,
    },
    global: { stubs: { teleport: true } },
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

async function dblClickNode(wrapper: ReturnType<typeof mountCanvas>, testid: string) {
  const card = wrapper.get(`[data-testid="${testid}"]`)
  await card.trigger('dblclick')
}

// Mirrors FlowsCanvas.vue's own port-position math (CARD_WIDTH/CARD_HEIGHT/
// PORT_HEIGHT, portTop for a single port) so tests can name a drop point in
// terms of a node's layout position rather than a hand-computed magic
// number. zoom is 1 and pan is {0,0} by default (no fit()/drag has run), and
// happy-dom reports getBoundingClientRect() as all-zero (see the fit() test
// below), so a world coordinate here is also the clientX/clientY to dispatch.
function portWorldPos(pos: { x: number; y: number }, output: boolean): { x: number; y: number } {
  const cardWidth = 176
  const cardHeight = 52
  const portHeight = 13
  const top = (cardHeight - portHeight) / 2
  return { x: output ? pos.x + cardWidth : pos.x, y: pos.y + top + portHeight / 2 }
}

/** Drags a wire from an output port's testid to a world/client point, then releases there. */
async function dragWire(wrapper: ReturnType<typeof mountCanvas>, fromTestId: string, to: { x: number; y: number }) {
  const port = wrapper.get(`[data-testid="${fromTestId}"]`)
  await port.trigger('pointerdown', { button: 0 })
  window.dispatchEvent(new PointerEvent('pointermove', { clientX: to.x, clientY: to.y }))
  await nextTick()
  window.dispatchEvent(new PointerEvent('pointerup', { clientX: to.x, clientY: to.y }))
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

  it('a single click (no drag) selects the node — no drawer, just the accent highlight ring', async () => {
    const wrapper = mountCanvas()

    await clickNode(wrapper, 'flow-node-filter')

    // Selection shows as an accent ring via cardShadow's box-shadow.
    const card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).toContain('var(--color-accent)')
    expect(wrapper.find('[data-testid="node-editor"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('clicking empty canvas space deselects the node', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-filter')
    let card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).toContain('var(--color-accent)')

    await wrapper.get('[data-testid="flows-canvas"]').trigger('click')

    card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).not.toContain('var(--color-accent)')

    wrapper.unmount()
  })

  it('a double click opens the NodeEditorDrawer for that node and selects it', async () => {
    const wrapper = mountCanvas()

    await dblClickNode(wrapper, 'flow-node-filter')

    expect(wrapper.find('[data-testid="node-editor-title"]').text()).toBe('Edit node · GitHub filter')

    wrapper.unmount()
  })

  it('dragging a node past the threshold emits move and does not select or open the drawer', async () => {
    const wrapper = mountCanvas()
    const card = wrapper.get('[data-testid="flow-node-source"]') // layout position {x:10, y:20}

    await card.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 })
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 130, clientY: 100 }))
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 130, clientY: 100 }))
    await nextTick()

    expect(wrapper.emitted('move')).toEqual([['source', 40, 20]])
    expect(wrapper.find('[data-testid="node-editor"]').exists()).toBe(false)
    const sourceCard = wrapper.get('[data-testid="flow-node-source"] > div')
    expect(sourceCard.attributes('style')).not.toContain('var(--color-accent)')

    wrapper.unmount()
  })

  it('a drawer save re-emits update-node and closes the drawer', async () => {
    const wrapper = mountCanvas()
    await dblClickNode(wrapper, 'flow-node-feed')

    await wrapper.get('[data-testid="node-editor-save"]').trigger('click')

    expect(wrapper.emitted('update-node')).toEqual([[{ id: 'feed', type: 'feed', disabled: false, config: { feed: 'inbox' } }]])
    expect(wrapper.find('[data-testid="node-editor"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('a drawer delete re-emits delete-node and closes the drawer', async () => {
    const wrapper = mountCanvas()
    await dblClickNode(wrapper, 'flow-node-feed')

    await wrapper.get('[data-testid="node-editor-delete"]').trigger('click')
    await wrapper.get('[data-testid="node-editor-delete-confirm"]').trigger('click')

    expect(wrapper.emitted('delete-node')).toEqual([['feed']])
    expect(wrapper.find('[data-testid="node-editor"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('a drawer close (Cancel) closes the drawer without emitting save or delete', async () => {
    const wrapper = mountCanvas()
    await dblClickNode(wrapper, 'flow-node-feed')

    await wrapper.get('[data-testid="node-editor-cancel"]').trigger('click')

    expect(wrapper.find('[data-testid="node-editor"]').exists()).toBe(false)
    expect(wrapper.emitted('update-node')).toBeUndefined()
    expect(wrapper.emitted('delete-node')).toBeUndefined()

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
    let card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).not.toContain('var(--color-accent)')

    await wrapper.setProps({ focusNodeId: 'filter' })

    card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).toContain('var(--color-accent)')
    // A single 176×52 node's bbox is tiny next to the (fallback) 1200×800
    // viewport, so fitToBBox clamps zoom to its 1.5 max.
    expect(wrapper.vm.zoom).toBe(1.5)

    wrapper.unmount()
  })

  it('dragging from an output port to a valid input port emits add-wire with the right {from, out, to}', async () => {
    // wires: [] — the default flow() already wires source->filter, which
    // would make this drop a (rejected) duplicate.
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })
    const filterInput = portWorldPos({ x: 400, y: 20 }, false)

    await dragWire(wrapper, 'port-out-source-0', filterInput)

    expect(wrapper.emitted('add-wire')).toEqual([[{ from: 'source', out: 0, to: 'filter' }]])

    wrapper.unmount()
  })

  it('shows a live draft wire and highlights the hovered valid target while dragging, clearing both on drop', async () => {
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })
    const filterInput = portWorldPos({ x: 400, y: 20 }, false)

    await wrapper.get('[data-testid="port-out-source-0"]').trigger('pointerdown', { button: 0 })
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: filterInput.x, clientY: filterInput.y }))
    await nextTick()

    expect(wrapper.find('[data-testid="wire-draft"]').exists()).toBe(true)
    const filterInputPort = wrapper.get('[data-testid="flow-node-filter"] [data-testid="port-in"]')
    expect(filterInputPort.classes()).toContain('port-target-valid')

    window.dispatchEvent(new PointerEvent('pointerup', { clientX: filterInput.x, clientY: filterInput.y }))
    await nextTick()

    expect(wrapper.find('[data-testid="wire-draft"]').exists()).toBe(false)
    expect(wrapper.emitted('add-wire')).toEqual([[{ from: 'source', out: 0, to: 'filter' }]])

    wrapper.unmount()
  })

  it('dropping on a node with no input port (a source) does not emit add-wire', async () => {
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })
    // A point inside source's 176x52 card bbox (source has no port-in at all).
    const overSource = { x: 10 + 40, y: 20 + 26 }

    await dragWire(wrapper, 'port-out-filter-0', overSource)

    expect(wrapper.emitted('add-wire')).toBeUndefined()

    wrapper.unmount()
  })

  it('dropping back on the same node does not emit add-wire (no self-connection)', async () => {
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })
    // A point inside filter's own card bbox — the same node the drag started from.
    const overFilter = { x: 400 + 40, y: 20 + 26 }

    await dragWire(wrapper, 'port-out-filter-0', overFilter)

    expect(wrapper.emitted('add-wire')).toBeUndefined()

    wrapper.unmount()
  })

  it('a port-drag does not select, move, or open the drawer for the card it started on (disambiguation)', async () => {
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })

    await dragWire(wrapper, 'port-out-filter-0', { x: 2000, y: 2000 }) // nowhere near any node — cancels

    expect(wrapper.emitted('add-wire')).toBeUndefined()
    expect(wrapper.emitted('move')).toBeUndefined()
    const card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).not.toContain('var(--color-accent)')
    expect(wrapper.find('[data-testid="node-editor"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('the wire-delete affordance emits remove-wire with the correct wire', async () => {
    const wrapper = mountCanvas() // default wires: source->filter, filter->feed

    await wrapper.get('[data-testid="wire-delete-0"]').trigger('click')

    expect(wrapper.emitted('remove-wire')).toEqual([[{ from: 'source', to: 'filter' }]])

    wrapper.unmount()
  })
})
