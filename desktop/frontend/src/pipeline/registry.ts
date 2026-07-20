// App registry (D2/Phase 6): palette metadata + editor + defaults, discovered
// from nodes/*/index.ts. Worker runtimes live in processors.ts so the app
// bundle never imports runtime.ts modules.

import type { NodeCategory, NodeTypeDefinition } from './nodeType'
import type { FlowNode } from './types'


const nodeModules = import.meta.glob<{ default: NodeTypeDefinition }>('./nodes/*/index.ts', { eager: true })

/** Every registered node type, keyed by its `type` string. */
export const byType: Record<string, NodeTypeDefinition> = {}

for (const path in nodeModules) {
  const def = nodeModules[path]?.default
  if (!def || !def.type) {
    throw new Error(`pipeline: ${path} does not default-export a NodeTypeDefinition with a "type"`)
  }
  if (byType[def.type]) {
    throw new Error(`pipeline: duplicate node type "${def.type}" (registered by ${path})`)
  }
  byType[def.type] = def
}

/** Palette entries grouped by category, in registration order within each group. */
export const palette: Record<NodeCategory, NodeTypeDefinition[]> = {
  Sources: [],
  Process: [],
  Destinations: [],
}

for (const def of Object.values(byType)) {
  palette[def.category].push(def)
}

// A short, readable, collision-avoiding id suffix — no Math.random (repo
// convention); a module-level monotonic counter is sufficient since node ids
// only need to be unique within one flow's canvas.
let idCounter = 0

export function genId(type: string): string {
  idCounter += 1
  return `${type}-${idCounter}`
}

/**
 * Builds a fresh FlowNode for a palette drag/drop. `defaults` is
 * deep-cloned so two instances of the same type never share config (a drag
 * of the same palette entry twice must not have editing one node's fields
 * silently edit the other's).
 */
export function instantiate(type: string): FlowNode {
  const def = byType[type]
  if (!def) throw new Error(`pipeline: unknown node type "${type}"`)
  return {
    id: genId(type),
    type,
    config: structuredClone(def.defaults),
  }
}
