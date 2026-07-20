// Converts between the engine's in-memory Flow/FlowNode (types.ts — a
// node's per-type fields nested under `config`) and the shape
// FlowsService.GetFlow/SaveFlow actually speak over Wails: flow.Flow's Node
// is flattened — envelope fields (id/type/name/disabled) and the per-type
// config's own fields all at the top level of one JSON object (see Go's
// node_json.go MarshalJSON/UnmarshalJSON). The generated binding types this
// as `Node = any` (a custom marshaler defeats struct-tag reflection), so
// there is nothing useful to import for the node shape itself — WireNode
// below is this module's own description of that flattened wire shape.
//
// Centralizing the conversion here — the only place in src/pipeline that
// imports these particular generated types — mirrors types.ts's posture for
// the engine's own wire types (Msg/CommitBatch/...): one file owns the
// import, everything else goes through the names declared here.
import type { Layout as WireLayoutModel, NodePosition } from '../../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/flow/models'
import type { FlowSummary } from '../../../bindings/github.com/colonyops/hive/desktop/models'
import type { NodeRunRecord } from '../../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'
import type { Flow, FlowNode, Wire } from '../types'

export type { FlowSummary, NodeRunRecord, NodePosition }
export type WireLayout = WireLayoutModel

/** The flattened per-node wire shape GetFlow/SaveFlow send/receive — see module docs above. */
export interface WireNode {
  id: string
  type: string
  name?: string
  disabled?: boolean
  [configField: string]: unknown
}

/** The flattened whole-flow wire shape GetFlow/SaveFlow send/receive. */
export interface WireFlow {
  id: string
  name: string
  enabled: boolean
  nodes: WireNode[] | null
  wires: Wire[] | null
}

/** The editor's working copy of one flow: the engine's Flow (id/nodes/wires) plus the two fields GetFlow/SaveFlow round-trip that the engine itself never needed (name, enabled). */
export interface EditorFlow extends Flow {
  name: string
  enabled: boolean
}

const ENVELOPE_KEYS = new Set(['id', 'type', 'name', 'disabled'])

/** Un-flattens one wire node into the engine's {id, type, name?, disabled?, config} shape. */
export function nodeFromWire(node: WireNode): FlowNode {
  const config: Record<string, any> = {}
  for (const [key, value] of Object.entries(node)) {
    if (!ENVELOPE_KEYS.has(key)) config[key] = value
  }
  return {
    id: node.id,
    type: node.type,
    ...(node.name ? { name: node.name } : {}),
    ...(node.disabled ? { disabled: node.disabled } : {}),
    config,
  }
}

/** Flattens one engine FlowNode back into the wire shape (config fields spread to the top level, alongside the envelope). */
export function nodeToWire(node: FlowNode): WireNode {
  return {
    id: node.id,
    type: node.type,
    ...(node.name ? { name: node.name } : {}),
    ...(node.disabled ? { disabled: node.disabled } : {}),
    ...node.config,
  }
}

/** Converts a GetFlow response into the editor's working EditorFlow. */
export function flowFromWire(wire: WireFlow): EditorFlow {
  return {
    id: wire.id,
    name: wire.name,
    enabled: wire.enabled,
    nodes: (wire.nodes ?? []).map(nodeFromWire),
    wires: wire.wires ?? [],
  }
}

/** Converts the editor's working EditorFlow into a SaveFlow request. */
export function flowToWire(flow: EditorFlow): WireFlow {
  return {
    id: flow.id,
    name: flow.name,
    enabled: flow.enabled,
    nodes: flow.nodes.map(nodeToWire),
    wires: flow.wires,
  }
}

/**
 * A deterministic default canvas position for the node at `index` among a
 * flow's nodes — a simple 4-per-row grid. Shared by usePipelineEditor's
 * addNode (a newly-added node needs a real, persisted position) and
 * FlowsCanvas's render-time fallback (a node loaded from a flow with no
 * matching .ui.yaml entry — e.g. hand-authored YAML — still needs somewhere
 * to draw, without that fallback being written back until the node is
 * actually moved).
 */
export function gridPosition(index: number): NodePosition {
  const perRow = 4
  const col = index % perRow
  const row = Math.floor(index / perRow)
  return { x: 80 + col * 260, y: 80 + row * 160 }
}

/**
 * Normalizes a GetLayout response (whose `nodes` map may come back null —
 * see the generated binding type) into one the editor can always index and
 * mutate directly (`layout.nodes[id] = ...`) without a null check at every
 * call site. Also used to build a fresh empty layout for a brand-new flow.
 */
export function normalizeLayout(layout?: WireLayout | null): WireLayout {
  return { nodes: layout?.nodes ? { ...layout.nodes } : {} }
}
