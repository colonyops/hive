// function is the author-trusted JS processor node (1 in / N out). This
// file is the single source of truth for its Config shape and the
// compile()/checkSyntax() helpers — runtime.ts (the worker-side
// ProcessorRuntime) and the future Phase 6 drawer's live syntax check both
// import from here rather than duplicating the `new Function(...)` call.

import type { Msg } from '../../types'
import type { NodeResult } from '../../engine/transport'

export const type = 'function'
export const role = 'processor' as const
// isolate:true — the engine (WebWorkerTransport) spawns a dedicated worker
// per instance of this type so a timeout's terminate() only kills this one
// node, never a sibling instance or the shared declarative-runtime worker.
export const isolate = true

export interface Config {
  /** required — the body of `on_message(msg, node, state)`. */
  on_message: string
  /** optional — runs once per instance before the first message. */
  on_start?: string
  /** optional — runs once per instance on teardown (Phase 4 Deploy drain). */
  on_stop?: string
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

export type CompiledFn = (msg: Msg, node: Record<string, any>, state: Record<string, any>) => NodeResult

/**
 * Compiles a JS body into a callable `(msg, node, state) => NodeResult`.
 * `new Function` is deliberate: the function node is author-trusted per the
 * design (D2) — no sandbox, the same posture Node-RED's own function node
 * takes. Construction (not just calling) throws a SyntaxError on invalid
 * source, which is what checkSyntax below relies on.
 */
export function compile(src: string): CompiledFn {
  // eslint-disable-next-line no-new-func
  return new Function('msg', 'node', 'state', src) as CompiledFn
}

/**
 * Surfaces syntax errors from `src` without running it — shared by the
 * (future) Phase 6 drawer's live syntax check. Returns an empty array when
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

// ── Phase 6: app-registry metadata (D2) ─────────────────────────────────────

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
  if (config.on_start) errors.push(...checkSyntax(config.on_start).map((e) => `on_start: ${e}`))
  if (config.on_stop) errors.push(...checkSyntax(config.on_stop).map((e) => `on_stop: ${e}`))
  if (config.outputs !== undefined && (config.outputs < 1 || config.outputs > 16)) {
    errors.push('outputs must be between 1 and 16')
  }
  if (config.timeout !== undefined && (config.timeout < 100 || config.timeout > 60000)) {
    errors.push('timeout must be between 100ms and 60s')
  }
  return errors
}
