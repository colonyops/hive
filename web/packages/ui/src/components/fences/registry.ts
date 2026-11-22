import { hasInjectionContext, shallowReactive, type Component } from 'vue'
import { useHiveUiPlatform } from '../../platform'

export interface FenceContext {
  docPath: string
  paths: ReadonlySet<string>
  /** Opens an href with the same rules as a markdown link (linkTarget), through the host's open-link. */
  follow(href: string): void
}

export interface FenceProps<T = unknown> {
  /** The fence body parsed as YAML and checked by the renderer's validate. */
  data: T
  /** The raw body, for Source and Copy. */
  source: string
  ctx: FenceContext
  testid: string
}

export interface FenceRenderer {
  /** Lowercase fence info string. */
  lang: string
  builtin: boolean
  /** Parse YAML into JSON (default), or hand over the raw text. Mermaid is the only built-in that takes text. */
  body: 'yaml' | 'text'
  /** Checks the parsed JSON and narrows it, or returns an error the host shows above the source. */
  validate?(data: unknown): { ok: true; data: unknown } | { ok: false; error: string }
  component: Component
}

// The registry is the one piece of shared state here on purpose: a register
// or unregister re-splits every open document through isFence.
// eslint-disable-next-line no-restricted-syntax -- see above
const renderers = shallowReactive(new Map<string, FenceRenderer>())

// Built-ins register at module load, before any app has installed the
// platform, so a rejection there has no logger to reach.
function warn(msg: string, fields: object): void {
  if (hasInjectionContext()) useHiveUiPlatform().log('fences').warn(msg, fields)
  // eslint-disable-next-line no-console -- no platform to log through before an app installs one
  else console.warn(`fences: ${msg}`, fields)
}

export function registerFence(renderer: FenceRenderer): () => void {
  const { lang } = renderer
  if (renderers.has(lang)) {
    warn('fence already registered', { lang })
    return () => {}
  }
  if (!renderer.builtin && lang.startsWith('hive-')) {
    warn('fence lang reserved for built-ins', { lang })
    return () => {}
  }
  renderers.set(lang, renderer)
  return () => {
    if (renderers.get(lang) === renderer) renderers.delete(lang)
  }
}

export function fenceRenderer(lang: string): FenceRenderer | undefined {
  return renderers.get(lang)
}

export function isFence(lang: string): boolean {
  return renderers.has(lang)
}
