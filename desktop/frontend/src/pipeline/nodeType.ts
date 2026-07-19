// The frontend node-type contract (D2): a plain frozen object describing one
// node type's palette metadata, default config, live UX validation, and
// drawer editor component. Each nodes/<type>/config.ts stays the single
// source of truth for that type's Config shape and pure helpers (D1/D2);
// index.ts wires config.ts + editor.vue + help.md into one of these via
// defineNodeType(), and registry.ts discovers every index.ts via
// import.meta.glob to build the app registry (palette + instantiate()).
//
// Modeled on Vue's own `defineComponent`/`defineStore` idiom — a plain frozen
// object, not a class (a node type has no encapsulated state; class
// instances don't survive postMessage/YAML round-trips) and not a
// composable (nothing here is reactive at the type layer).

export type NodeRole = 'source' | 'processor' | 'output'
export type NodeCategory = 'Sources' | 'Process' | 'Destinations'

export interface NodeTypeDefinition<C = Record<string, any>> {
  /** The one cross-boundary string: YAML `type:`, Go registry key, worker registry key. */
  type: string
  label: string
  category: NodeCategory
  /** source -> runs in Go (F2); processor -> Web Worker; output -> engine-collected commit intent (sink). */
  role: NodeRole
  /** Icon component (unplugin-icons `~icons/lucide/*`). */
  glyph: any
  defaults: C
  /** Fixed port count, or a function of config (e.g. the function node's `outputs?`). */
  outputs?: number | ((c: C) => number)
  /** UX-only live validation for the drawer — Go's SaveFlow validator is authoritative. */
  validate?(c: C): string[]
  /** Drawer body — a controlled component: props {config: C, errors?: string[]}, emits update:config (immutable). */
  editor: any
  /** Raw help.md contents (imported `?raw`), rendered via lib/markdown.ts. */
  help: string
}

export function defineNodeType<C>(def: NodeTypeDefinition<C>): NodeTypeDefinition<C> {
  return Object.freeze(def)
}
