<script setup lang="ts">
// Functional-but-basic flows canvas (Phase 6b): node cards at their layout
// position (falling back to a deterministic grid slot when unpositioned —
// see lib/wireFlow.ts's gridPosition), SVG wires between ports, drag-to-
// reposition, a simple zoom/fit, live per-node status from nodeRuns, and a
// click-to-open NodeEditorDrawer (reused from Phase 6a) for add/edit/delete.
//
// Wire *creation* is intentionally out of scope for v1 (see help text in
// the empty state and the module docs on NodePalette.vue/FlowsView.vue):
// existing wires render and can be removed, but drawing a new one by
// pointer is fiddly enough that it's left to hand-editing the flow's YAML
// for now — node add/edit/delete, layout, and Deploy are the must-haves.
import { computed, ref } from 'vue'
import { byType } from '../registry'
import { hasInputPort, outputPortCount } from '../lib/ports'
import { gridPosition, type EditorFlow, type NodePosition, type NodeRunRecord, type WireLayout } from '../lib/wireFlow'
import type { FlowNode, Wire } from '../types'
import NodeEditorDrawer from './NodeEditorDrawer.vue'

const props = defineProps<{
  flow: EditorFlow
  layout: WireLayout
  latestRunByNode: Map<string, NodeRunRecord>
}>()

const emit = defineEmits<{
  move: [id: string, x: number, y: number]
  'update-node': [node: FlowNode]
  'delete-node': [id: string]
}>()

const CARD_WIDTH = 208
const CARD_HEIGHT = 64

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

function cardStyle(node: FlowNode) {
  const pos = positions.value.get(node.id) ?? { x: 0, y: 0 }
  return { transform: `translate(${pos.x}px, ${pos.y}px)`, width: `${CARD_WIDTH}px` }
}

function roleClasses(node: FlowNode): string {
  const role = defFor(node)?.role
  if (role === 'source') return 'bg-kind-pr-tint text-kind-pr'
  if (role === 'output') return 'bg-feeds/15 text-feeds'
  return 'bg-accent-tint text-accent'
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

/** Vertical offset (px, card-relative) of port `index` of `total` evenly spaced ports down the card's height. */
function portOffset(index: number, total: number): number {
  if (total <= 1) return CARD_HEIGHT / 2
  const step = CARD_HEIGHT / (total + 1)
  return step * (index + 1)
}

function portStyle(index: number, total: number) {
  return { top: `${portOffset(index, total)}px` }
}

/** World-space (canvas-content) coordinates of one port, for wire drawing. */
function portPoint(nodeId: string, portIndex: number, output: boolean): { x: number; y: number } {
  const node = nodesById.value.get(nodeId)
  const pos = positions.value.get(nodeId)
  if (!node || !pos) return { x: 0, y: 0 }
  const def = defFor(node)
  const total = output ? (def ? outputPortCount(def, node) : 1) : 1
  return {
    x: output ? pos.x + CARD_WIDTH : pos.x,
    y: pos.y + portOffset(portIndex, Math.max(total, 1)),
  }
}

function wirePath(wire: Wire): string {
  const from = portPoint(wire.from, wire.out ?? 0, true)
  const to = portPoint(wire.to, 0, false)
  const bend = Math.max(60, Math.abs(to.x - from.x) / 2)
  return `M ${from.x} ${from.y} C ${from.x + bend} ${from.y}, ${to.x - bend} ${to.y}, ${to.x} ${to.y}`
}

// ── Live status ──────────────────────────────────────────────────────────

type RunStatus = 'idle' | 'ok' | 'error'

function runStatus(nodeId: string): RunStatus {
  const run = props.latestRunByNode.get(nodeId)
  if (!run) return 'idle'
  return run.ok ? 'ok' : 'error'
}

function statusClasses(nodeId: string): string {
  const status = runStatus(nodeId)
  if (status === 'ok') return 'text-severity-success'
  if (status === 'error') return 'text-severity-error'
  return 'text-text-4'
}

function statusLabel(nodeId: string): string {
  const run = props.latestRunByNode.get(nodeId)
  if (!run) return 'idle'
  if (!run.ok) return run.err || 'error'
  return `${run.inCount}→${run.outCount} · ${ageLabel(run.endedAt)}`
}

function cardClasses(node: FlowNode): string {
  const status = runStatus(node.id)
  if (status === 'error') return 'border-severity-error shadow-[0_0_0_3px_var(--color-severity-error-tint)]'
  if (selectedNodeId.value === node.id) return 'border-accent'
  return 'border-card'
}

// endedAt is stored as Go's time.UnixNano() — convert to ms for Date.now() comparisons.
function ageLabel(endedAtNano: number): string {
  const ms = Date.now() - endedAtNano / 1e6
  if (ms < 1000) return 'just now'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  return `${h}h ago`
}

// ── Drag to reposition / click to open the drawer ───────────────────────
// A pointerdown starts tracking; if the pointer moves past a small
// threshold before pointerup, it's a drag (moveNode fires continuously);
// otherwise it's a click (opens the drawer) — the same disambiguation
// FeedList-style clickable rows don't need, but a draggable canvas does.

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

// ── Zoom / fit — a basic scale transform; panning only happens as a side
// effect of Fit centering content, there's no click-drag-to-pan in v1. ────

const viewportRef = ref<HTMLElement | null>(null)
const zoom = ref(1)
const pan = ref({ x: 0, y: 0 })

function zoomIn() {
  zoom.value = Math.min(2, Math.round((zoom.value + 0.1) * 100) / 100)
}
function zoomOut() {
  zoom.value = Math.max(0.2, Math.round((zoom.value - 0.1) * 100) / 100)
}

function fit() {
  if (props.flow.nodes.length === 0) {
    zoom.value = 1
    pan.value = { x: 0, y: 0 }
    return
  }
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const pos of positions.value.values()) {
    minX = Math.min(minX, pos.x)
    minY = Math.min(minY, pos.y)
    maxX = Math.max(maxX, pos.x + CARD_WIDTH)
    maxY = Math.max(maxY, pos.y + CARD_HEIGHT)
  }
  const contentWidth = Math.max(1, maxX - minX)
  const contentHeight = Math.max(1, maxY - minY)
  const viewportWidth = viewportRef.value?.clientWidth || 1200
  const viewportHeight = viewportRef.value?.clientHeight || 800
  const padding = 48
  const scale = Math.min((viewportWidth - padding * 2) / contentWidth, (viewportHeight - padding * 2) / contentHeight)
  zoom.value = Math.min(1.5, Math.max(0.25, scale))
  pan.value = { x: padding - minX * zoom.value, y: padding - minY * zoom.value }
}

// ── Node editor drawer (Phase 6a) ────────────────────────────────────────

const selectedNodeId = ref<string | null>(null)
const selectedNode = computed(() => props.flow.nodes.find((n) => n.id === selectedNodeId.value) ?? null)
const selectedDef = computed(() => (selectedNode.value ? byType[selectedNode.value.type] : null))

function onDrawerSave(node: FlowNode) {
  emit('update-node', node)
  selectedNodeId.value = null
}

function onDrawerDelete(id: string) {
  emit('delete-node', id)
  selectedNodeId.value = null
}
</script>

<template>
  <div ref="viewportRef" class="relative h-full w-full overflow-hidden bg-app" data-testid="flows-canvas">
    <div class="absolute right-3 top-3 z-10 flex items-center gap-1 rounded-lg border border-strong bg-pane/90 p-1">
      <button class="cursor-pointer rounded px-2 py-1 text-[13px] text-text-2 hover:text-text" data-testid="canvas-zoom-out" @click="zoomOut">−</button>
      <span class="w-11 text-center text-[11px] text-text-3" data-testid="canvas-zoom-level">{{ Math.round(zoom * 100) }}%</span>
      <button class="cursor-pointer rounded px-2 py-1 text-[13px] text-text-2 hover:text-text" data-testid="canvas-zoom-in" @click="zoomIn">+</button>
      <div class="mx-1 h-4 w-px bg-row" />
      <button class="cursor-pointer rounded px-2 py-1 text-[11.5px] text-text-2 hover:text-text" data-testid="canvas-fit" @click="fit">Fit</button>
    </div>

    <div class="absolute bottom-3 left-3 z-10 flex items-center gap-3 rounded-lg border border-strong bg-pane/90 px-2.5 py-1.5 text-[10.5px] text-text-3" data-testid="canvas-legend">
      <span class="flex items-center gap-1.5"><span class="size-1.5 rounded-full bg-text-4" />Idle</span>
      <span class="flex items-center gap-1.5"><span class="size-1.5 rounded-full bg-severity-success" />OK</span>
      <span class="flex items-center gap-1.5"><span class="size-1.5 rounded-full bg-severity-error" />Error</span>
    </div>

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
          stroke-width="2"
          data-testid="flow-wire"
        />
      </svg>

      <div
        v-for="node in flow.nodes"
        :key="node.id"
        class="absolute cursor-grab touch-none select-none rounded-xl border bg-raised p-2.5 shadow-sm active:cursor-grabbing"
        :class="cardClasses(node)"
        :style="cardStyle(node)"
        :data-testid="`flow-node-${node.id}`"
        @pointerdown="onNodePointerDown($event, node)"
      >
        <div class="flex items-center gap-2">
          <span class="flex size-6 shrink-0 items-center justify-center rounded-md" :class="roleClasses(node)">
            <component :is="defFor(node)?.glyph" class="size-3.5" />
          </span>
          <div class="min-w-0 flex-1">
            <div class="truncate text-[12.5px] font-semibold text-text" data-testid="flow-node-title">{{ node.name || defFor(node)?.label || node.type }}</div>
            <div class="truncate text-[10px] text-text-3">{{ node.type }}</div>
          </div>
        </div>
        <div class="mt-1.5 truncate text-[10.5px]" :class="statusClasses(node.id)" data-testid="flow-node-status">{{ statusLabel(node.id) }}</div>

        <span v-if="hasInput(node)" class="port-dot absolute -left-[5px]" :style="portStyle(0, 1)" data-testid="port-in" />
        <span
          v-for="p in outputPorts(node)"
          :key="p"
          class="port-dot absolute -right-[5px]"
          :style="portStyle(p, outputPorts(node).length)"
          :data-testid="`port-out-${node.id}-${p}`"
        />
      </div>
    </div>

    <NodeEditorDrawer
      v-if="selectedNode && selectedDef"
      :node="selectedNode"
      :def="selectedDef"
      @save="onDrawerSave"
      @delete="onDrawerDelete"
      @close="selectedNodeId = null"
    />
  </div>
</template>

<style scoped>
.port-dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: var(--color-strong);
  border: 2px solid var(--color-app);
  transform: translateY(-50%);
}
</style>
