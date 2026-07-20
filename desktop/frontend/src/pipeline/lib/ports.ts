// Port-count helpers shared by anything that needs to draw a node's ports
// (currently just FlowsCanvas.vue) — the closed-form resolution of
// NodeTypeDefinition.outputs (a fixed number, a function of config, or
// undefined meaning "1") into an actual count, plus whether a node has an
// input port at all.

import type { NodeTypeDefinition } from '../nodeType'
import type { FlowNode, Wire } from '../types'

/** Whether def's nodes accept an input wire — only source nodes (backend-fed) have none. */
export function hasInputPort(def: NodeTypeDefinition): boolean {
  return def.role !== 'source'
}

/** The number of output ports a node has — 0 for output/terminal nodes, def.outputs resolved against the node's own config otherwise (defaulting to 1 when the type declares none). */
export function outputPortCount(def: NodeTypeDefinition, node: FlowNode): number {
  if (def.role === 'output') return 0
  if (typeof def.outputs === 'function') return def.outputs(node.config)
  if (typeof def.outputs === 'number') return def.outputs
  return 1
}

/**
 * Whether a wire from `fromNode`'s output port `fromPortIndex` to `toNode`'s
 * (sole) input is legal — the single rule set FlowsCanvas.vue's drag-to-wire
 * gesture enforces both live (to gate the input-port "drop to connect"
 * highlight, 8c) and on drop (to gate the actual add-wire emit), so the two
 * never disagree:
 *
 *  - no self-connection (a node can't wire to itself)
 *  - both node types must resolve via `defForType` (an unknown type is never
 *    a valid endpoint)
 *  - the target must accept input at all (`hasInputPort` — source nodes,
 *    backend-fed, never do)
 *  - `fromPortIndex` must be one of the source node's actual output ports
 *  - the exact wire (same from/out/to) must not already exist in
 *    `existingWires` — mirrors usePipelineEditor.ts's own addWire dedup, so
 *    a highlighted port never leads to a silently-dropped duplicate emit.
 *
 * Pure and registry-agnostic: `defForType` is injected (production passes
 * `byType`; tests pass a small fake) rather than importing the registry
 * directly, keeping this unit-testable without pulling in every node type.
 */
export function canConnect(
  fromNode: FlowNode,
  fromPortIndex: number,
  toNode: FlowNode,
  existingWires: Wire[],
  defForType: (type: string) => NodeTypeDefinition | undefined,
): boolean {
  if (fromNode.id === toNode.id) return false
  const fromDef = defForType(fromNode.type)
  const toDef = defForType(toNode.type)
  if (!fromDef || !toDef) return false
  if (!hasInputPort(toDef)) return false
  if (fromPortIndex < 0 || fromPortIndex >= outputPortCount(fromDef, fromNode)) return false
  return !existingWires.some((w) => w.from === fromNode.id && (w.out ?? 0) === fromPortIndex && w.to === toNode.id)
}
