// The worker registry: runtime type -> ProcessorRuntime, discovered via a
// Vite glob over every node type's runtime.ts. This is the *worker* half of
// D2's two-registry split (the other half — an app/palette registry over
// index.ts — is below). vitest supports import.meta.glob directly, so this
// doubles as the registry both InProcessTransport (via tests/the fallback)
// and a real worker bundle entry (production) would load.

import type { ProcessorRuntime } from './engine/transport'
import type { NodeCategory, NodeTypeDefinition } from './nodeType'
import type { FlowNode } from './types'

const modules = import.meta.glob<{ default: ProcessorRuntime }>('./nodes/*/runtime.ts', { eager: true })

export const processorRegistry: Record<string, ProcessorRuntime> = {}

for (const path in modules) {
  const runtime = modules[path]?.default
  if (!runtime || !runtime.type) {
    throw new Error(`pipeline: ${path} does not default-export a ProcessorRuntime with a "type"`)
  }
  if (processorRegistry[runtime.type]) {
    throw new Error(`pipeline: duplicate runtime type "${runtime.type}" (registered by ${path})`)
  }
  processorRegistry[runtime.type] = runtime
}

// ── App registry (D2/Phase 6): the *frontend* half of the two-registry
// split, over nodes/*/index.ts (palette metadata + editor + defaults) rather
// than nodes/*/runtime.ts (worker execution, above). Discovered the same
// way, via import.meta.glob, so adding a node type means adding a directory
// — no registry file to hand-edit.

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
