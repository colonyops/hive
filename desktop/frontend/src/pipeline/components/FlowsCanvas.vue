<script setup lang="ts">
// The flows canvas (8a/8c): node cards at their layout position (falling
// back to a deterministic grid slot when unpositioned — see
// lib/wireFlow.ts's gridPosition), SVG wires between ports, drag-to-
// reposition, zoom/fit (exposed for FlowsView's toolbar — see defineExpose
// below), live per-node status from nodeRuns, and a single-click-selects /
// double-click-opens-the-NodeEditorDrawer flow for add/edit/delete. There is
// no floating inspector — selection is just the accent highlight ring
// (cardShadow) on the selected card; opening the editor is a distinct,
// explicit double-click gesture.
//
// Wire creation (8c "WIRING"): a pointerdown on an *output* port
// (@pointerdown.stop.prevent — stopping the card's own onNodePointerDown
// from also firing is the port-drag-vs-card-drag disambiguation) starts a
// drag tracked in `wireDraft`; a live dashed SVG path follows the pointer
// (world coords via clientToWorld, mirroring onNodePointerDown's zoom-scaled
// delta math below) and any input port the pointer is currently over that
// lib/ports.ts's canConnect() accepts gets the "drop to connect" highlight.
// pointerup over a valid target emits add-wire; anywhere else cancels.
// Wire deletion is hover-driven: each rendered wire gets an invisible wide
// hit-path plus a small ✕ control at its midpoint, both revealed via the
// scoped .wire-group:hover rules below, emitting remove-wire on click.
// FlowsView.vue binds both emits straight to usePipelineEditor's
// addWire/removeWire.
import { computed, ref, watch } from 'vue'
import { byType } from '../registry'
import { canConnect, hasInputPort, outputPortCount } from '../lib/ports'
import { classify, statusColor, statusLabel, statusPulses } from '../lib/runStatus'
import { gridPosition, type EditorFlow, type NodePosition, type NodeRunRecord, type WireLayout } from '../lib/wireFlow'
import type { FlowNode, Wire } from '../types'
import NodeEditorDrawer from './NodeEditorDrawer.vue'

const props = defineProps<{
  flow: EditorFlow
  layout: WireLayout
  latestRunByNode: Map<string, NodeRunRecord>
  /**
   * Node ids currently mid-execution — the 'running' status (8c: blue
   * pulsing) has no real per-node signal yet (node_run rows are only
   * written for a *completed* pump); this is plumbing for whenever a caller
   * has one (see lib/runStatus.ts's module docs). Defaults to none running.
   */
  runningNodeIds?: Set<string>
  /** A node to select + center-pan on (e.g. a "reveal in flow" deep link). Reuses fit()'s bbox/scale/pan mechanism on a single-node bbox. */
  focusNodeId?: string | null
}>()

const emit = defineEmits<{
  move: [id: string, x: number, y: number]
  'update-node': [node: FlowNode]
  'delete-node': [id: string]
  'add-wire': [wire: Wire]
  'remove-wire': [wire: Wire]
}>()

const CARD_WIDTH = 176
const CARD_HEIGHT = 52
const PORT_WIDTH = 9
const PORT_HEIGHT = 13

// ── Positions — layout wins, a deterministic grid slot fills in for a node
// with no saved position (e.g. hand-authored YAML with no .ui.yaml yet). ───

const positions = computed<Map<string, NodePosition>>(() => {
  const map = new Map<string, NodePosition>()
  props.flow.nodes.forEach((node, index) => {
    map.set(node.id, props.layout.nodes?.[node.id] ?? gridPosition(index))
  })
  return map
})

const nodesById = computed(() => new Map(props.flow.nodes.map((n) => [n.id, n])))

function defFor(node: FlowNode) {
  return byType[node.type]
}

function titleFor(node: FlowNode): string {
  return node.name || defFor(node)?.label || node.type
}

function cardWrapperStyle(node: FlowNode) {
  const pos = positions.value.get(node.id) ?? { x: 0, y: 0 }
  return { transform: `translate(${pos.x}px, ${pos.y}px)`, width: `${CARD_WIDTH}px` }
}

function capColor(node: FlowNode): string {
  return defFor(node)?.accentToken ?? 'var(--color-accent)'
}

function tintColor(node: FlowNode): string {
  return defFor(node)?.tint ?? 'var(--color-accent-tint)'
}

function cardShadow(node: FlowNode): string {
  const status = statusFor(node)
  if (status === 'error') return '0 0 0 1.5px var(--color-severity-error)'
  if (selectedNodeId.value === node.id) return '0 0 0 1.5px var(--color-accent), 0 6px 16px -9px rgba(0, 0, 0, .5)'
  return '0 6px 16px -9px rgba(0, 0, 0, .5)'
}

// ── Ports ────────────────────────────────────────────────────────────────

function outputPorts(node: FlowNode): number[] {
  const def = defFor(node)
  if (!def) return []
  const count = outputPortCount(def, node)
  return Array.from({ length: count }, (_, i) => i)
}

function hasInput(node: FlowNode): boolean {
  const def = defFor(node)
  return !!def && hasInputPort(def)
}

/** Top offset (px, card-relative) of port `index` of `total` evenly spaced 9×13 ports down the card's height. */
function portTop(index: number, total: number): number {
  if (total <= 1) return (CARD_HEIGHT - PORT_HEIGHT) / 2
  const step = CARD_HEIGHT / (total + 1)
  return step * (index + 1) - PORT_HEIGHT / 2
}

function portStyle(index: number, total: number) {
  return { top: `${portTop(index, total)}px`, width: `${PORT_WIDTH}px`, height: `${PORT_HEIGHT}px` }
}

/** World-space (canvas-content) coordinates of one port's center, for wire drawing. */
function portPoint(nodeId: string, portIndex: number, output: boolean): { x: number; y: number } {
  const node = nodesById.value.get(nodeId)
  const pos = positions.value.get(nodeId)
  if (!node || !pos) return { x: 0, y: 0 }
  const def = defFor(node)
  const total = output ? (def ? outputPortCount(def, node) : 1) : 1
  return {
    x: output ? pos.x + CARD_WIDTH : pos.x,
    y: pos.y + portTop(portIndex, Math.max(total, 1)) + PORT_HEIGHT / 2,
  }
}

/** Cubic-bezier `d` between two world-space points — shared by wirePath (a real wire), draftPath (the live in-progress drag), and nothing else needs the bend math duplicated. */
function bezierPath(from: { x: number; y: number }, to: { x: number; y: number }): string {
  const bend = Math.max(60, Math.abs(to.x - from.x) / 2)
  return `M ${from.x} ${from.y} C ${from.x + bend} ${from.y}, ${to.x - bend} ${to.y}, ${to.x} ${to.y}`
}

function wirePath(wire: Wire): string {
  return bezierPath(portPoint(wire.from, wire.out ?? 0, true), portPoint(wire.to, 0, false))
}

/** Midpoint of a wire's two port points — where its hover delete (✕) control sits. */
function wireMidpoint(wire: Wire): { x: number; y: number } {
  const from = portPoint(wire.from, wire.out ?? 0, true)
  const to = portPoint(wire.to, 0, false)
  return { x: (from.x + to.x) / 2, y: (from.y + to.y) / 2 }
}

// ── Live status (8c: idle / running / done / error) ─────────────────────

function statusFor(node: FlowNode) {
  return classify(props.latestRunByNode.get(node.id), props.runningNodeIds?.has(node.id) ?? false)
}

function statusDotColor(node: FlowNode): string {
  return statusColor(statusFor(node))
}

function statusText(node: FlowNode): string {
  return statusLabel(statusFor(node), props.latestRunByNode.get(node.id))
}

function statusTextColor(node: FlowNode): string {
  const status = statusFor(node)
  if (status === 'error') return 'var(--color-severity-error)'
  if (status === 'running') return 'var(--color-severity-running)'
  if (status === 'ok') return 'var(--color-text-2)'
  return 'var(--color-text-4)'
}

function statusDotPulses(node: FlowNode): boolean {
  return statusPulses(statusFor(node))
}

// ── Drag to reposition / click to select / double-click to open the editor ──
// A pointerdown starts tracking; if the pointer moves past a small
// threshold before pointerup, it's a drag (moveNode fires continuously) and
// never selects or opens anything. Otherwise it's a click: a single click
// selects the node (accent highlight ring via cardShadow); a second click
// within the double-click window opens the NodeEditorDrawer instead.

const DRAG_THRESHOLD = 4

function onNodePointerDown(e: PointerEvent, node: FlowNode) {
  if (e.button !== 0) return
  e.preventDefault()
  const startClientX = e.clientX
  const startClientY = e.clientY
  const origin = positions.value.get(node.id) ?? { x: 0, y: 0 }
  let moved = false

  function onMove(ev: PointerEvent) {
    const dx = (ev.clientX - startClientX) / zoom.value
    const dy = (ev.clientY - startClientY) / zoom.value
    if (Math.abs(ev.clientX - startClientX) > DRAG_THRESHOLD || Math.abs(ev.clientY - startClientY) > DRAG_THRESHOLD) {
      moved = true
    }
    emit('move', node.id, Math.round(origin.x + dx), Math.round(origin.y + dy))
  }
  function onUp() {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    if (!moved) selectedNodeId.value = node.id
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

function onNodeDblClick(node: FlowNode) {
  selectedNodeId.value = node.id
  drawerOpen.value = true
}

function onSurfaceClick() {
  selectedNodeId.value = null
}

// ── Wire creation by drag (8c "WIRING") ─────────────────────────────────
// A pointerdown on an output port (not the card) starts a drag: wireDraft
// tracks the source node/port and the pointer's current world position, so
// the template can render a live dashed path (draftPath) from the source
// port to the cursor. On every move, nodeAt() hit-tests the drag's current
// world position against each node's card bbox (a full-card target is a
// larger, easier-to-hit drop zone than the 9×13 port itself) and
// canConnect() (lib/ports.ts) gates whether that node is actually a legal
// target — the same gate used again on drop, so the "drop to connect"
// highlight (hoverTargetId) and the eventual add-wire emit can never
// disagree.

interface WireDraft {
  fromNodeId: string
  fromPort: number
  toX: number
  toY: number
}

const wireDraft = ref<WireDraft | null>(null)
const hoverTargetId = ref<string | null>(null)

/** Converts a pointer event's client coords into world (canvas-content) coords — the inverse of the outer content div's `translate(pan) scale(zoom)` transform, mirroring onNodePointerDown's zoom-scaled delta math above but as an absolute position rather than a delta. */
function clientToWorld(clientX: number, clientY: number): { x: number; y: number } {
  const rect = viewportRef.value?.getBoundingClientRect()
  const left = rect?.left ?? 0
  const top = rect?.top ?? 0
  return { x: (clientX - left - pan.value.x) / zoom.value, y: (clientY - top - pan.value.y) / zoom.value }
}

/** The node whose 176×52 card bbox contains a world-space point, if any. */
function nodeAt(worldX: number, worldY: number): FlowNode | null {
  for (const node of props.flow.nodes) {
    const pos = positions.value.get(node.id)
    if (!pos) continue
    if (worldX >= pos.x && worldX <= pos.x + CARD_WIDTH && worldY >= pos.y && worldY <= pos.y + CARD_HEIGHT) return node
  }
  return null
}

function onOutputPortPointerDown(e: PointerEvent, node: FlowNode, portIndex: number) {
  if (e.button !== 0) return
  const start = portPoint(node.id, portIndex, true)
  wireDraft.value = { fromNodeId: node.id, fromPort: portIndex, toX: start.x, toY: start.y }
  hoverTargetId.value = null

  function targetAt(clientX: number, clientY: number): FlowNode | null {
    const world = clientToWorld(clientX, clientY)
    const target = nodeAt(world.x, world.y)
    return target && canConnect(node, portIndex, target, props.flow.wires, (type) => byType[type]) ? target : null
  }

  function onMove(ev: PointerEvent) {
    const world = clientToWorld(ev.clientX, ev.clientY)
    wireDraft.value = { fromNodeId: node.id, fromPort: portIndex, toX: world.x, toY: world.y }
    hoverTargetId.value = targetAt(ev.clientX, ev.clientY)?.id ?? null
  }
  function onUp(ev: PointerEvent) {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    const target = targetAt(ev.clientX, ev.clientY)
    if (target) emit('add-wire', { from: node.id, out: portIndex, to: target.id })
    wireDraft.value = null
    hoverTargetId.value = null
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

const draftPath = computed<string>(() => {
  const draft = wireDraft.value
  if (!draft) return ''
  return bezierPath(portPoint(draft.fromNodeId, draft.fromPort, true), { x: draft.toX, y: draft.toY })
})

// ── Zoom / fit — a basic scale transform; panning only happens as a side
// effect of Fit/focus centering content, there's no click-drag-to-pan in
// v1. The buttons themselves live in FlowsView's canvas toolbar (8a) —
// zoom/zoomIn/zoomOut/fit are exposed below for that toolbar to drive. ────

const viewportRef = ref<HTMLElement | null>(null)
const zoom = ref(1)
const pan = ref({ x: 0, y: 0 })

function zoomIn() {
  zoom.value = Math.min(2, Math.round((zoom.value + 0.1) * 100) / 100)
}
function zoomOut() {
  zoom.value = Math.max(0.2, Math.round((zoom.value - 0.1) * 100) / 100)
}

interface BBox { minX: number; minY: number; maxX: number; maxY: number }

function bboxOf(ids: string[]): BBox | null {
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  let found = false
  for (const id of ids) {
    const pos = positions.value.get(id)
    if (!pos) continue
    found = true
    minX = Math.min(minX, pos.x)
    minY = Math.min(minY, pos.y)
    maxX = Math.max(maxX, pos.x + CARD_WIDTH)
    maxY = Math.max(maxY, pos.y + CARD_HEIGHT)
  }
  return found ? { minX, minY, maxX, maxY } : null
}

/** Scales+pans so `bbox` fills the viewport (with padding), clamped to [0.25, 1.5] zoom. Shared by fit() (whole-flow bbox) and centerOnNode() (single-node bbox) — see its call below. */
function fitToBBox(bbox: BBox) {
  const contentWidth = Math.max(1, bbox.maxX - bbox.minX)
  const contentHeight = Math.max(1, bbox.maxY - bbox.minY)
  const viewportWidth = viewportRef.value?.clientWidth || 1200
  const viewportHeight = viewportRef.value?.clientHeight || 800
  const padding = 48
  const scale = Math.min((viewportWidth - padding * 2) / contentWidth, (viewportHeight - padding * 2) / contentHeight)
  zoom.value = Math.min(1.5, Math.max(0.25, scale))
  pan.value = { x: padding - bbox.minX * zoom.value, y: padding - bbox.minY * zoom.value }
}

function fit() {
  const bbox = bboxOf(props.flow.nodes.map((n) => n.id))
  if (!bbox) {
    zoom.value = 1
    pan.value = { x: 0, y: 0 }
    return
  }
  fitToBBox(bbox)
}

/** Selects `id` and center-pans on it alone — reuses fit()'s bbox/scale/pan mechanism (fitToBBox) on a single-node bbox instead of the whole flow's. */
function centerOnNode(id: string) {
  const bbox = bboxOf([id])
  if (!bbox) return
  selectedNodeId.value = id
  fitToBBox(bbox)
}

watch(() => props.focusNodeId, (id) => {
  if (id) centerOnNode(id)
}, { immediate: true })

defineExpose({ zoom, zoomIn, zoomOut, fit })

// ── Selection: single click selects (accent ring), double click opens the
// NodeEditorDrawer ────────────────────────────────────────────────────────

const selectedNodeId = ref<string | null>(null)
const selectedNode = computed(() => props.flow.nodes.find((n) => n.id === selectedNodeId.value) ?? null)
const selectedDef = computed(() => (selectedNode.value ? byType[selectedNode.value.type] : null))
const drawerOpen = ref(false)

function onDrawerSave(node: FlowNode) {
  emit('update-node', node)
  drawerOpen.value = false
}

function onDrawerDelete(id: string) {
  emit('delete-node', id)
  drawerOpen.value = false
  selectedNodeId.value = null
}
</script>

<template>
  <div
    ref="viewportRef"
    class="relative h-full w-full overflow-hidden"
    data-testid="flows-canvas"
    :style="{
      backgroundColor: 'var(--color-canvas)',
      backgroundImage: 'radial-gradient(var(--color-canvas-dot) 1.1px, transparent 1.1px)',
      backgroundSize: '22px 22px',
      backgroundPosition: '-1px -1px',
    }"
    @click.self="onSurfaceClick"
  >
    <div v-if="flow.nodes.length === 0" class="flex h-full items-center justify-center px-8 text-center text-[13px] text-text-4" data-testid="canvas-empty">
      Add a node from the palette to get started. Drag from an output port to an input port to wire nodes together.
    </div>

    <div class="absolute left-0 top-0 origin-top-left" :style="{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})` }">
      <svg class="pointer-events-none absolute left-0 top-0 h-0 w-0 overflow-visible">
        <g v-for="(wire, i) in flow.wires" :key="i" class="wire-group">
          <path
            :d="wirePath(wire)"
            fill="none"
            class="wire-visible"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            data-testid="flow-wire"
          />
          <path
            :d="wirePath(wire)"
            fill="none"
            stroke="transparent"
            stroke-width="16"
            class="wire-hitbox"
          />
          <g
            :transform="`translate(${wireMidpoint(wire).x}, ${wireMidpoint(wire).y})`"
            class="wire-delete"
            :data-testid="`wire-delete-${i}`"
            @click.stop="emit('remove-wire', wire)"
          >
            <circle r="8" class="wire-delete-bg" />
            <path d="M-3,-3 L3,3 M3,-3 L-3,3" class="wire-delete-x" />
          </g>
        </g>

        <path
          v-if="wireDraft"
          :d="draftPath"
          fill="none"
          stroke="var(--color-accent)"
          stroke-width="2.5"
          stroke-linecap="round"
          stroke-dasharray="6 6"
          data-testid="wire-draft"
        />
      </svg>

      <div
        v-for="node in flow.nodes"
        :key="node.id"
        class="absolute cursor-grab touch-none select-none"
        :style="cardWrapperStyle(node)"
        :data-testid="`flow-node-${node.id}`"
        @pointerdown="onNodePointerDown($event, node)"
        @dblclick="onNodeDblClick(node)"
      >
        <div class="relative flex h-[52px] overflow-hidden rounded-[2px] bg-action-card active:cursor-grabbing" :style="{ boxShadow: cardShadow(node) }">
          <div class="w-1.5 shrink-0" :style="{ background: capColor(node) }" />
          <div class="flex min-w-0 flex-1 items-center gap-2.5 px-[11px]">
            <span class="flex size-[23px] shrink-0 items-center justify-center rounded-md" :style="{ background: tintColor(node), color: capColor(node) }">
              <component :is="defFor(node)?.glyph" class="size-3.5" />
            </span>
            <div class="min-w-0 flex-1">
              <div class="truncate text-[12.5px] font-semibold text-text" data-testid="flow-node-title">{{ titleFor(node) }}</div>
              <div class="truncate font-mono text-[10.5px] text-text-3">{{ node.type }}</div>
            </div>
          </div>

          <span
            v-if="hasInput(node)"
            class="port absolute -left-[5px]"
            :class="{ 'port-target-valid': wireDraft && hoverTargetId === node.id }"
            :style="portStyle(0, 1)"
            data-testid="port-in"
          />
          <span
            v-for="p in outputPorts(node)"
            :key="p"
            class="port absolute -right-[5px] cursor-crosshair"
            :style="portStyle(p, outputPorts(node).length)"
            :data-testid="`port-out-${node.id}-${p}`"
            @pointerdown.stop.prevent="onOutputPortPointerDown($event, node, p)"
          />
        </div>

        <div v-if="wireDraft && hoverTargetId === node.id" class="drop-hint" data-testid="wire-drop-hint">drop to connect</div>

        <div class="mt-1.5 flex items-center gap-1.5 pl-[3px]">
          <span class="size-2 shrink-0 rounded-full" :class="{ 'hive-pulse': statusDotPulses(node) }" :style="{ background: statusDotColor(node) }" />
          <span class="truncate font-mono text-[10.5px]" :style="{ color: statusTextColor(node) }" data-testid="flow-node-status">{{ statusText(node) }}</span>
        </div>
      </div>
    </div>

    <NodeEditorDrawer
      v-if="selectedNode && selectedDef && drawerOpen"
      :node="selectedNode"
      :def="selectedDef"
      @save="onDrawerSave"
      @delete="onDrawerDelete"
      @close="drawerOpen = false"
    />
  </div>
</template>

<style scoped>
.port {
  border-radius: 3px;
  background: var(--color-node-port);
  border: 1px solid var(--color-canvas);
}

/* 8c "drop to connect": the currently-hovered legal wire-drag target. */
.port-target-valid {
  background: var(--color-accent);
  box-shadow: 0 0 0 3px rgba(245, 158, 11, 0.32);
}

.drop-hint {
  position: absolute;
  left: -2px;
  top: -24px;
  white-space: nowrap;
  pointer-events: none;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 10px;
  color: var(--color-accent);
  background: var(--color-pane);
  border: 1px solid var(--color-accent);
  border-radius: 5px;
  padding: 2px 7px;
}

.hive-pulse {
  animation: hivePulse 1.6s ease-in-out infinite;
}

/* Hover-to-delete a wire (8c): the thin visible path never itself catches
   pointer events (fill:none + a 2.5px stroke is too small to hit reliably);
   the invisible wire-hitbox path underneath does, and :hover on it — via
   plain CSS ancestor propagation — reveals wire-delete and recolors
   wire-visible on the shared wire-group. */
.wire-visible {
  stroke: var(--color-strong);
  transition: stroke 120ms ease;
}

.wire-hitbox {
  pointer-events: stroke;
  cursor: pointer;
}

.wire-delete {
  pointer-events: auto;
  cursor: pointer;
  opacity: 0;
  transition: opacity 120ms ease;
}

.wire-delete-bg {
  fill: var(--color-severity-error);
}

.wire-delete-x {
  stroke: white;
  stroke-width: 1.5px;
  stroke-linecap: round;
}

.wire-group:hover .wire-visible {
  stroke: var(--color-severity-error);
}

.wire-group:hover .wire-delete {
  opacity: 1;
}
</style>
