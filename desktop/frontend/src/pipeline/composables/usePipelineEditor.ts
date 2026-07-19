// Editor state over FlowsService/PipelineService (Phase 6b): the flows
// picker's summaries, the selected flow's draft graph + layout, dirty
// tracking for Deploy, and polled node_run status for the canvas. Mirrors
// driver.ts's injection posture — the composable core never imports
// bindings/ or @wailsio/runtime directly; the mounting component
// (FlowsView.vue) adapts the real generated bindings into
// PipelineEditorClient and is also the one place that subscribes to the
// "flows:updated" Wails event (calling refreshFlows/refreshNodeRuns here in
// response), keeping this file trivially unit-testable with a fake client.
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { instantiate } from '../registry'
import type { FlowNode, Wire } from '../types'
import {
  flowFromWire,
  flowToWire,
  gridPosition,
  normalizeLayout,
  type EditorFlow,
  type FlowSummary,
  type NodePosition,
  type NodeRunRecord,
  type WireFlow,
  type WireLayout,
} from '../lib/wireFlow'

export type { EditorFlow }

/**
 * The subset of FlowsService/PipelineService the editor needs. A thin
 * adapter over the generated GetFlow/SaveFlow/GetLayout/SaveLayout/
 * ListFlows/NodeRuns bindings satisfies this shape — see FlowsView.vue.
 */
export interface PipelineEditorClient {
  listFlows(): Promise<FlowSummary[] | null | undefined>
  getFlow(id: string): Promise<WireFlow>
  saveFlow(flow: WireFlow): Promise<void>
  getLayout(id: string): Promise<WireLayout>
  saveLayout(id: string, layout: WireLayout): Promise<void>
  nodeRuns(flowId: string, limit: number): Promise<NodeRunRecord[] | null | undefined>
}

export interface PipelineEditorOptions {
  /** node_run poll interval while a flow is selected. Default 5000ms. */
  pollIntervalMs?: number
  /** Recent node_run rows to keep — enough for a RECENT list plus one row per node. Default 100. */
  nodeRunLimit?: number
}

export function usePipelineEditor(client: PipelineEditorClient, options: PipelineEditorOptions = {}) {
  const pollIntervalMs = options.pollIntervalMs ?? 5000
  const nodeRunLimit = options.nodeRunLimit ?? 100

  const flows = ref<FlowSummary[]>([])
  const activeFlow = ref<EditorFlow | null>(null)
  const layout = ref<WireLayout>(normalizeLayout())
  // True once the selected flow has local edits GetFlow/GetLayout wouldn't
  // reproduce — the only thing gating Deploy.
  const dirty = ref(false)
  const nodeRuns = ref<NodeRunRecord[]>([])

  const loadingFlows = ref(false)
  const loadingFlow = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  /**
   * Latest node_run per node id. nodeRuns is newest-first (the backend
   * query orders by ended_at DESC), so the first occurrence per node id
   * encountered while scanning is already its latest run.
   */
  const latestRunByNode = computed(() => {
    const map = new Map<string, NodeRunRecord>()
    for (const run of nodeRuns.value) {
      if (!map.has(run.nodeId)) map.set(run.nodeId, run)
    }
    return map
  })

  async function refreshFlows(): Promise<void> {
    loadingFlows.value = true
    try {
      flows.value = (await client.listFlows()) ?? []
    } catch (err) {
      console.warn('Unable to load flows', err)
      error.value = message(err, 'Could not load flows.')
    } finally {
      loadingFlows.value = false
    }
  }

  async function refreshNodeRuns(): Promise<void> {
    if (!activeFlow.value) {
      nodeRuns.value = []
      return
    }
    try {
      nodeRuns.value = (await client.nodeRuns(activeFlow.value.id, nodeRunLimit)) ?? []
    } catch (err) {
      // Node-run status is a live overlay on top of the canvas, not a
      // blocking load — log and keep whatever status was already shown
      // rather than surfacing an error for a background poll tick.
      console.debug('Unable to load node runs', err)
    }
  }

  async function selectFlow(id: string): Promise<void> {
    loadingFlow.value = true
    error.value = null
    try {
      const [wireFlow, wireLayout] = await Promise.all([client.getFlow(id), client.getLayout(id)])
      activeFlow.value = flowFromWire(wireFlow)
      layout.value = normalizeLayout(wireLayout)
      dirty.value = false
      await refreshNodeRuns()
    } catch (err) {
      console.warn('Unable to load flow', id, err)
      error.value = message(err, 'Could not load the flow.')
      activeFlow.value = null
    } finally {
      loadingFlow.value = false
    }
  }

  /**
   * Starts a brand-new flow as a local, unsaved draft — nothing is written
   * until deploy(). The id is a slug derived from name, de-duplicated
   * against the known flows list; Go's FlowStore.Save creates the file the
   * first time a flow with that id is saved, so no backend call is needed
   * up front. An optimistic row is added to `flows` so the picker shows the
   * new flow immediately; deploy()'s refreshFlows() replaces it with the
   * real saved summary.
   */
  function newFlow(name: string): void {
    const id = uniqueId(slugify(name), new Set(flows.value.map((f) => f.id)))
    activeFlow.value = { id, name, enabled: true, nodes: [], wires: [] }
    layout.value = normalizeLayout()
    nodeRuns.value = []
    error.value = null
    dirty.value = true
    flows.value = [...flows.value, { id, name, enabled: true, valid: true }]
  }

  function requireFlow(): EditorFlow {
    if (!activeFlow.value) throw new Error('pipeline: no active flow selected')
    return activeFlow.value
  }

  /** Instantiates type from the registry, appends it to the draft flow, and places it at a default canvas position. */
  function addNode(type: string): FlowNode {
    const flow = requireFlow()
    const node = instantiate(type)
    flow.nodes = [...flow.nodes, node]
    setNodePosition(node.id, gridPosition(flow.nodes.length - 1))
    dirty.value = true
    return node
  }

  function updateNode(node: FlowNode): void {
    const flow = requireFlow()
    flow.nodes = flow.nodes.map((n) => (n.id === node.id ? node : n))
    dirty.value = true
  }

  function deleteNode(id: string): void {
    const flow = requireFlow()
    flow.nodes = flow.nodes.filter((n) => n.id !== id)
    flow.wires = flow.wires.filter((w) => w.from !== id && w.to !== id)
    const nodes = { ...(layout.value.nodes ?? {}) }
    delete nodes[id]
    layout.value = { ...layout.value, nodes }
    dirty.value = true
  }

  function addWire(wire: Wire): void {
    const flow = requireFlow()
    if (flow.wires.some((w) => sameWire(w, wire))) return
    flow.wires = [...flow.wires, wire]
    dirty.value = true
  }

  function removeWire(wire: Wire): void {
    const flow = requireFlow()
    flow.wires = flow.wires.filter((w) => !sameWire(w, wire))
    dirty.value = true
  }

  function moveNode(id: string, x: number, y: number): void {
    setNodePosition(id, { x, y })
    dirty.value = true
  }

  function setNodePosition(id: string, pos: NodePosition): void {
    layout.value = { ...layout.value, nodes: { ...(layout.value.nodes ?? {}), [id]: pos } }
  }

  async function deploy(): Promise<void> {
    const flow = requireFlow()
    saving.value = true
    error.value = null
    try {
      await client.saveFlow(flowToWire(flow))
      await client.saveLayout(flow.id, layout.value)
      dirty.value = false
      await refreshFlows()
    } catch (err) {
      console.warn('Unable to deploy flow', flow.id, err)
      error.value = message(err, 'Could not deploy the flow.')
    } finally {
      saving.value = false
    }
  }

  let pollTimer: ReturnType<typeof setInterval> | undefined

  onMounted(() => {
    void refreshFlows()
    pollTimer = setInterval(() => { void refreshNodeRuns() }, pollIntervalMs)
  })

  onUnmounted(() => {
    if (pollTimer !== undefined) clearInterval(pollTimer)
  })

  return {
    flows,
    activeFlow,
    layout,
    dirty,
    nodeRuns,
    latestRunByNode,
    loadingFlows,
    loadingFlow,
    saving,
    error,
    refreshFlows,
    refreshNodeRuns,
    selectFlow,
    newFlow,
    addNode,
    updateNode,
    deleteNode,
    addWire,
    removeWire,
    moveNode,
    deploy,
  }
}

// ── helpers ──────────────────────────────────────────────────────────────

function sameWire(a: Wire, b: Wire): boolean {
  return a.from === b.from && (a.out ?? 0) === (b.out ?? 0) && a.to === b.to
}

function slugify(name: string): string {
  const base = name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  return base.slice(0, 64) || 'flow'
}

function uniqueId(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base
  let n = 2
  while (taken.has(`${base}-${n}`)) n++
  return `${base}-${n}`
}

function message(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback
}
