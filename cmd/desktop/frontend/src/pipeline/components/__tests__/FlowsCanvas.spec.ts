import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import FlowsCanvas from '../FlowsCanvas.vue'
import { NODE_TYPE_MIME } from '../../lib/dragTypes'
import type { EditorFlow, NodeRunRecord, WireLayout } from '../../lib/wireFlow'

function flow(overrides: Partial<EditorFlow> = {}): EditorFlow {
  return {
    id: 'flow-1',
    name: 'My flow',
    enabled: true,
    nodes: [
      { id: 'source', type: 'sources.github', config: { source: 'my-prs' } },
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
  return { flowId: 'flow-1', nodeId: 'source', ok: true, inCount: 3, outCount: 3, dropCount: 0, err: '', durMs: 5, endedAt: Date.now(), ...overrides }
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

/**
 * A point clear of every card in flow()/layout() — and of the grab margin
 * around them, which is what a press on the canvas surface is tested against.
 * The cards occupy y 20–132 across x 10–776.
 */
const EMPTY_SPACE = { clientX: 100, clientY: 300 }

async function pressCanvas(wrapper: ReturnType<typeof mountCanvas>, at: { clientX: number; clientY: number }) {
  await wrapper.get('[data-testid="flows-canvas"]').trigger('pointerdown', { button: 0, ...at })
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

/** The inline px geometry of one output port's transparent grab target. */
function grabBox(wrapper: ReturnType<typeof mountCanvas>, nodeId: string, port: number) {
  const style = wrapper.get(`[data-testid="port-grab-${nodeId}-${port}"]`).attributes('style') ?? ''
  const px = (prop: string) => Number(new RegExp(`\\b${prop}:\\s*(-?[\\d.]+)px`).exec(style)?.[1])
  return { top: px('top'), height: px('height'), right: px('right'), width: px('width') }
}

/** Drags a wire from an output port's grab target to a world/client point, then releases there. */
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
    const wrapper = mountCanvas({ flow: flow({ nodes: [{ id: 'source', type: 'sources.github', name: 'My PRs', config: { source: 'my-prs' } }] }) })

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

  // The drawn port is a 9×13 rect hung 5px off a card that clips its overflow,
  // leaving a 4px sliver to aim at. The grab target is a separate, transparent
  // sibling of the card — outside the clip, straddling the edge, and centred on
  // the same point the wire anchors to.
  it('gives each output port a 20px grab target straddling the card edge, clamped so stacked ports do not overlap', () => {
    const wrapper = mountCanvas({
      flow: flow({ nodes: [{ id: 'one', type: 'sources.github', config: {} }, { id: 'two', type: 'github-filter', config: {} }], wires: [] }),
    })

    // A lone port sits at the card's vertical centre (26), so a 20px box spans 16–36.
    const single = grabBox(wrapper, 'one', 0)
    expect(single).toMatchObject({ width: 20, right: -10, height: 20, top: 16 })

    // github-filter has two ports, their centres 52/3 ≈ 17.33 apart. Each
    // target stays centred on its own port and clamps to that spacing rather
    // than the full 20, so the two never overlap and steal each other's clicks.
    const first = grabBox(wrapper, 'two', 0)
    const second = grabBox(wrapper, 'two', 1)
    expect(first.height).toBeCloseTo(52 / 3)
    expect(first.top + first.height / 2).toBeCloseTo(52 / 3)
    expect(second.top + second.height / 2).toBeCloseTo((52 / 3) * 2)
    expect(first.top + first.height).toBeLessThanOrEqual(second.top)

    wrapper.unmount()
  })

  // A source card carries its product's hue and renders the product's own
  // logomark at tile size; downstream nodes keep the role hue and the 14px
  // lucide glyph. Asserted here (not just on the registry) because the tile is
  // where a brand is actually visible to someone scanning a canvas.
  it('paints a source card with its brand hue and a full-tile mark', () => {
    const wrapper = mountCanvas({
      flow: flow({
        nodes: [
          { id: 'source', type: 'sources.github', config: {} },
          { id: 'grafana', type: 'sources.grafana_metrics', config: {} },
          { id: 'hook', type: 'sources.webhook', config: {} },
          { id: 'feed', type: 'feed', config: {} },
        ],
        wires: [],
      }),
    })

    const github = wrapper.get('[data-testid="flow-node-source"] [data-testid="flow-node-mark"]')
    expect(github.attributes('style')).toContain('var(--color-brand-github-tint)')
    expect(github.attributes('style')).toContain('var(--color-brand-github)')
    expect(github.classes()).toContain('size-[26px]')

    const grafana = wrapper.get('[data-testid="flow-node-grafana"] [data-testid="flow-node-mark"]')
    expect(grafana.attributes('style')).toContain('var(--color-brand-grafana)')

    // A source with no vendor behind it keeps the generic hue and the lucide
    // tile — the bigger tile is for logomarks, not for sources as a class.
    const hook = wrapper.get('[data-testid="flow-node-hook"] [data-testid="flow-node-mark"]')
    expect(hook.attributes('style')).toContain('var(--color-node-blue)')
    expect(hook.classes()).toContain('size-[23px]')

    // Downstream nodes are unchanged: role hue, 23px tile, 14px glyph.
    const feed = wrapper.get('[data-testid="flow-node-feed"] [data-testid="flow-node-mark"]')
    expect(feed.attributes('style')).toContain('var(--color-node-green)')
    expect(feed.classes()).toContain('size-[23px]')

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

    await pressCanvas(wrapper, EMPTY_SPACE)
    window.dispatchEvent(new PointerEvent('pointerup', EMPTY_SPACE))
    await nextTick()

    card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).not.toContain('var(--color-accent)')

    wrapper.unmount()
  })

  // GRAB_MARGIN's slack: a press that lands just short of a card still grabs
  // the node rather than panning the canvas out from under it.
  it('pressing within the grab margin of a card drags that node instead of panning', async () => {
    const wrapper = mountCanvas() // source card spans x 10–186, y 20–72
    const content = wrapper.get('[data-testid="canvas-content"]')

    await pressCanvas(wrapper, { clientX: 190, clientY: 76 }) // 4px off the card's bottom-right corner
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 220, clientY: 76 }))
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 220, clientY: 76 }))
    await nextTick()

    expect(wrapper.emitted('move')).toEqual([['source', 40, 20]])
    expect(content.attributes('style')).toContain('translate(0px, 0px)')

    wrapper.unmount()
  })

  it('pressing beyond the grab margin pans, leaving the nearby node alone', async () => {
    const wrapper = mountCanvas()
    const content = wrapper.get('[data-testid="canvas-content"]')

    await pressCanvas(wrapper, { clientX: 200, clientY: 90 }) // 14px clear of the source card
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 230, clientY: 90 }))
    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 230, clientY: 90 }))
    await nextTick()

    expect(wrapper.emitted('move')).toBeUndefined()
    expect(content.attributes('style')).toContain('translate(30px, 0px)')

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

  it('click-dragging empty canvas space pans the graph with a grabbing cursor', async () => {
    const wrapper = mountCanvas()
    const canvas = wrapper.get('[data-testid="flows-canvas"]')
    const content = wrapper.get('[data-testid="canvas-content"]')

    await pressCanvas(wrapper, EMPTY_SPACE)
    expect(canvas.classes()).toContain('cursor-grabbing')

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 145, clientY: 330 }))
    await nextTick()
    expect(content.attributes('style')).toContain('translate(45px, 30px)')

    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 145, clientY: 330 }))
    await nextTick()
    expect(canvas.classes()).not.toContain('cursor-grabbing')

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 200, clientY: 400 }))
    await nextTick()
    expect(content.attributes('style')).toContain('translate(45px, 30px)')

    wrapper.unmount()
  })

  it('scrolling pans the canvas, with Shift converting a mouse wheel to horizontal pan', async () => {
    const wrapper = mountCanvas()
    const canvas = wrapper.get('[data-testid="flows-canvas"]')
    const content = wrapper.get('[data-testid="canvas-content"]')

    await canvas.trigger('wheel', { deltaX: 20, deltaY: 30 })
    expect(content.attributes('style')).toContain('translate(-20px, -30px)')

    await canvas.trigger('wheel', { deltaX: 0, deltaY: 25, shiftKey: true })
    expect(content.attributes('style')).toContain('translate(-45px, -30px)')

    wrapper.unmount()
  })

  it('Ctrl/Cmd+scroll zooms around the pointer location', async () => {
    const wrapper = mountCanvas()
    const canvas = wrapper.get('[data-testid="flows-canvas"]')
    const content = wrapper.get('[data-testid="canvas-content"]')

    await canvas.trigger('wheel', { clientX: 100, clientY: 80, deltaY: -100, ctrlKey: true })

    expect(wrapper.vm.zoom).toBeCloseTo(Math.exp(0.2))
    const transform = content.attributes('style') ?? ''
    expect(transform).toContain('translate(-22.')
    expect(transform).toContain('scale(1.221')

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

    await dragWire(wrapper, 'port-grab-source-0', filterInput)

    expect(wrapper.emitted('add-wire')).toEqual([[{ from: 'source', out: 0, to: 'filter' }]])

    wrapper.unmount()
  })

  it('shows a live draft wire and highlights the hovered valid target while dragging, clearing both on drop', async () => {
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })
    const filterInput = portWorldPos({ x: 400, y: 20 }, false)

    await wrapper.get('[data-testid="port-grab-source-0"]').trigger('pointerdown', { button: 0 })
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

    await dragWire(wrapper, 'port-grab-filter-0', overSource)

    expect(wrapper.emitted('add-wire')).toBeUndefined()

    wrapper.unmount()
  })

  it('dropping back on the same node does not emit add-wire (no self-connection)', async () => {
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })
    // A point inside filter's own card bbox — the same node the drag started from.
    const overFilter = { x: 400 + 40, y: 20 + 26 }

    await dragWire(wrapper, 'port-grab-filter-0', overFilter)

    expect(wrapper.emitted('add-wire')).toBeUndefined()

    wrapper.unmount()
  })

  it('a port-drag does not select, move, or open the drawer for the card it started on (disambiguation)', async () => {
    const wrapper = mountCanvas({ flow: flow({ wires: [] }) })

    await dragWire(wrapper, 'port-grab-filter-0', { x: 2000, y: 2000 }) // nowhere near any node — cancels

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

  it('Backspace deletes the selected node and clears the selection', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-filter')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Backspace' }))
    await nextTick()

    expect(wrapper.emitted('delete-node')).toEqual([['filter']])
    const card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).not.toContain('var(--color-accent)')

    wrapper.unmount()
  })

  it('Delete deletes the selected node and clears the selection', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-filter')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Delete' }))
    await nextTick()

    expect(wrapper.emitted('delete-node')).toEqual([['filter']])
    const card = wrapper.get('[data-testid="flow-node-filter"] > div')
    expect(card.attributes('style')).not.toContain('var(--color-accent)')

    wrapper.unmount()
  })

  it('does not emit delete-node on Backspace/Delete when nothing is selected', async () => {
    const wrapper = mountCanvas()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Backspace' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Delete' }))
    await nextTick()

    expect(wrapper.emitted('delete-node')).toBeUndefined()

    wrapper.unmount()
  })

  it('does not emit delete-node on Backspace when a text input has focus', async () => {
    const wrapper = mountCanvas()
    await clickNode(wrapper, 'flow-node-filter')

    const input = document.createElement('input')
    document.body.appendChild(input)
    input.focus()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Backspace' }))
    await nextTick()

    expect(wrapper.emitted('delete-node')).toBeUndefined()

    input.remove()
    wrapper.unmount()
  })

  it('does not emit delete-node on Backspace while the NodeEditorDrawer is open', async () => {
    const wrapper = mountCanvas()
    await dblClickNode(wrapper, 'flow-node-filter') // selects + opens the drawer

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Backspace' }))
    await nextTick()

    expect(wrapper.emitted('delete-node')).toBeUndefined()
    expect(wrapper.find('[data-testid="node-editor-title"]').exists()).toBe(true) // drawer still open

    wrapper.unmount()
  })

  it('dropping a palette node type emits add-node-at with world coords for the drop point', async () => {
    const wrapper = mountCanvas()
    const canvas = wrapper.get('[data-testid="flows-canvas"]')

    await canvas.trigger('drop', {
      clientX: 250,
      clientY: 90,
      dataTransfer: { getData: (fmt: string) => (fmt === NODE_TYPE_MIME ? 'feed' : '') },
    })

    // zoom is 1 and pan is {0,0} by default (no fit()/drag has run), and
    // happy-dom reports getBoundingClientRect() as all-zero, so the world
    // coords for this drop equal the client coords — mirrors dragWire's
    // portWorldPos convention above.
    expect(wrapper.emitted('add-node-at')).toEqual([['feed', 250, 90]])

    wrapper.unmount()
  })

  it('ignores a drop with no recognized node type in dataTransfer', async () => {
    const wrapper = mountCanvas()
    const canvas = wrapper.get('[data-testid="flows-canvas"]')

    await canvas.trigger('drop', { clientX: 250, clientY: 90, dataTransfer: { getData: () => '' } })

    expect(wrapper.emitted('add-node-at')).toBeUndefined()

    wrapper.unmount()
  })

  it('toggles a drop-target highlight while a drag is over the canvas, clearing it on drop', async () => {
    const wrapper = mountCanvas()
    const canvas = wrapper.get('[data-testid="flows-canvas"]')

    await canvas.trigger('dragenter', { dataTransfer: { types: [NODE_TYPE_MIME] } })
    expect(canvas.classes()).toContain('canvas-drop-target')

    await canvas.trigger('drop', { clientX: 0, clientY: 0, dataTransfer: { getData: () => 'feed' } })
    expect(canvas.classes()).not.toContain('canvas-drop-target')

    wrapper.unmount()
  })

  it('toggles the drop-target highlight off on dragleave', async () => {
    const wrapper = mountCanvas()
    const canvas = wrapper.get('[data-testid="flows-canvas"]')

    await canvas.trigger('dragenter', { dataTransfer: { types: [NODE_TYPE_MIME] } })
    expect(canvas.classes()).toContain('canvas-drop-target')

    await canvas.trigger('dragleave')
    expect(canvas.classes()).not.toContain('canvas-drop-target')

    wrapper.unmount()
  })
})
