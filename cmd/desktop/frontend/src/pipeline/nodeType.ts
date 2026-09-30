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
  /**
   * True when `glyph` is a product logomark rather than a lucide glyph.
   * Logomarks carry their own aspect and optical weight — PostHog's is 52:28 —
   * so they are drawn full-bleed in a slightly larger tile, where a glyph
   * from the 24×24 icon grid needs a fixed 14px optical size to sit right
   * next to every other node's.
   */
  logoMark?: boolean
  /**
   * Per-type accent color for the canvas card's role cap + glyph icon (a CSS
   * color value, e.g. `var(--color-node-blue)` — see styles/main.css's
   * node-* tokens). Falls back to the generic `var(--color-accent)` when
   * unset.
   */
  accentToken?: string
  /**
   * Per-type tinted background for the glyph tile (a CSS color value, e.g.
   * `var(--color-node-blue-tint)`). Falls back to the generic
   * `var(--color-accent-tint)` when unset.
   */
  tint?: string
  defaults: C
  /**
   * Optional per-instantiation seed, merged over `defaults` when a node is
   * dragged onto the canvas. For config that must differ between two nodes
   * of the same type — a webhook source's endpoint path — rather than being
   * a shared constant. `defaults` stays the static, promptable shape (the
   * flow LLM prompt serializes it); this only fills fields in.
   */
  freshConfig?(): Partial<C>
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
