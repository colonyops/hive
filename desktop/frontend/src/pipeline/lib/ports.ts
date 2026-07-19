// Port-count helpers shared by anything that needs to draw a node's ports
// (currently just FlowsCanvas.vue) — the closed-form resolution of
// NodeTypeDefinition.outputs (a fixed number, a function of config, or
// undefined meaning "1") into an actual count, plus whether a node has an
// input port at all.

import type { NodeTypeDefinition } from '../nodeType'
import type { FlowNode } from '../types'

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
