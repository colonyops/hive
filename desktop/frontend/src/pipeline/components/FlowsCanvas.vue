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
// Wire *creation* is intentionally out of scope here (a separate task):
// existing wires render, but drawing/removing one by pointer is left to
// hand-editing the flow's YAML for now — see NodePalette.vue's module docs
// for the same posture.
import { computed, ref, watch } from 'vue'
import { byType } from '../registry'
import { hasInputPort, outputPortCount } from '../lib/ports'
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

function wirePath(wire: Wire): string {
  const from = portPoint(wire.from, wire.out ?? 0, true)
  const to = portPoint(wire.to, 0, false)
  const bend = Math.max(60, Math.abs(to.x - from.x) / 2)
  return `M ${from.x} ${from.y} C ${from.x + bend} ${from.y}, ${to.x - bend} ${to.y}, ${to.x} ${to.y}`
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
      Add a node from the palette to get started. Wires render here once nodes are wired in the flow's YAML.
    </div>

    <div class="absolute left-0 top-0 origin-top-left" :style="{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})` }">
      <svg class="pointer-events-none absolute left-0 top-0 h-0 w-0 overflow-visible">
        <path
          v-for="(wire, i) in flow.wires"
          :key="i"
          :d="wirePath(wire)"
          fill="none"
          stroke="var(--color-strong)"
          stroke-width="2.5"
          stroke-linecap="round"
          stroke-linejoin="round"
          data-testid="flow-wire"
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

          <span v-if="hasInput(node)" class="port absolute -left-[5px]" :style="portStyle(0, 1)" data-testid="port-in" />
          <span
            v-for="p in outputPorts(node)"
            :key="p"
            class="port absolute -right-[5px]"
            :style="portStyle(p, outputPorts(node).length)"
            :data-testid="`port-out-${node.id}-${p}`"
          />
        </div>

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

.hive-pulse {
  animation: hivePulse 1.6s ease-in-out infinite;
}
</style>
