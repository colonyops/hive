// function is the author-trusted JS processor node (1 in / N out). This file
// is the editor's half: its Config shape, its palette metadata, and the
// compile()/checkSyntax() helpers behind the drawer's live syntax check.
//
// The script is executed by Go (internal/app/runtime/js), through goja. The
// two compilers are different engines, so this check is a fast local warning
// about obvious syntax errors, not the authority — SaveFlow is, and it
// compiles with the engine that will actually run the script.

export const type = 'function'
export const role = 'processor' as const

export interface Config {
  /**
   * required — the body of `on_message(msg, node, state, kv)`, and the node's
   * whole lifecycle. There are no start/stop hooks: the node does no I/O, so
   * setup belongs in `on_message` as lazy init (`state.counts ??= {}`).
   */
  on_message: string
  /** 1..16, default 1 (D1). */
  outputs?: number
  /** ms, 100..60000, default 5000 (D1). */
  timeout?: number
}

export const DEFAULT_OUTPUTS = 1
export const DEFAULT_TIMEOUT_MS = 5000

export function outputs(config: Config): number {
  return config.outputs ?? DEFAULT_OUTPUTS
}

export function timeoutMs(config: Config): number {
  return config.timeout ?? DEFAULT_TIMEOUT_MS
}

export type CompiledFn = (msg: unknown, node: Record<string, any>, state: Record<string, any>, kv: Record<string, any>) => unknown

/**
 * Compiles a JS body so a syntax error surfaces while typing. Nothing calls
 * the result — execution is Go's. `new Function` is used because construction
 * (not just calling) throws a SyntaxError on invalid source, which is the
 * whole point of checkSyntax below.
 */
export function compile(src: string): CompiledFn {
  // eslint-disable-next-line no-new-func
  return new Function('msg', 'node', 'state', 'kv', src) as CompiledFn
}

/**
 * Surfaces syntax errors from `src` without running it — shared by config
 * validation and the drawer's live syntax check. Returns an empty array when
 * `src` compiles cleanly.
 */
export function checkSyntax(src: string): string[] {
  try {
    compile(src)
    return []
  } catch (error) {
    return [error instanceof Error ? error.message : String(error)]
  }
}

/**
 * Copy-paste starting points for the canonical dedup intents, faithful to the
 * real runtime surface: `msg.Payload` is a live object (never JSON.parse it),
 * and keys use the full source-identity tuple — two sources feeding one node
 * can emit the same `msg.Key` for different items.
 */
export const recipes = [
  {
    id: 'once',
    label: 'Notify once, ever',
    code: `// Notify once, ever
const k = JSON.stringify([msg.SourceKind, msg.SourceScope, msg.Key])
if (kv.has(k)) return null
kv.set(k, true)
return msg`,
  },
  {
    id: 'on-change',
    label: 'Only on meaningful change',
    code: `// Notify only on a meaningful change (you pick the fields)
const k = JSON.stringify([msg.SourceKind, msg.SourceScope, msg.Key])
const p = msg.Payload || {}
const meaningful = JSON.stringify({ state: p.state, review: p.review_decision, sha: p.head_sha })
if (kv.get(k) === meaningful) return null
kv.set(k, meaningful)
return msg`,
  },
  {
    id: 'rate-limit',
    label: 'At most once per 4h',
    code: `// At most once per 4h per item (rate-limit / re-arm)
const k = JSON.stringify([msg.SourceKind, msg.SourceScope, msg.Key])
if (kv.has(k)) return null
kv.set(k, true, { ttl: 4 * 60 * 60 })
return msg`,
  },
] as const

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Function'
export const category = 'Process' as const
// Purple — matches the mockup's Function node cap (8c anatomy diagram).
export const accentToken = 'var(--color-node-purple)'
export const tint = 'var(--color-node-purple-tint)'

export const defaults: Config = {
  on_message: 'return msg',
}

/** UX-only — Go's SaveFlow validator is authoritative; this mirrors it for live drawer feedback. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!config.on_message || !config.on_message.trim()) {
    errors.push('on_message is required')
  } else {
    errors.push(...checkSyntax(config.on_message))
  }
  if (config.outputs !== undefined && (config.outputs < 1 || config.outputs > 16)) {
    errors.push('outputs must be between 1 and 16')
  }
  if (config.timeout !== undefined && (config.timeout < 100 || config.timeout > 60000)) {
    errors.push('timeout must be between 100ms and 60s')
  }
  return errors
}
