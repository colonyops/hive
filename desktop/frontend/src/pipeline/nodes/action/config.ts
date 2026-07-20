// action is a terminal node (1 in / 0 out): the arriving msg enqueues an
// output_command against the referenced desktop actions.yml action id. See
// nodes/feed/config.ts for the parallel terminal (sink/unread are the
// engine's single source of truth for commit-tagging a terminal node).

import IconZap from '~icons/lucide/zap'
import type { Sink } from '../../types'

export const type = 'action'
export const role = 'output' as const

export interface Config {
  /** action id in desktop actions.yml. */
  action: string
}

/** Action outputs are enqueued commands, not feed items — unread has no meaning here. */
export const unread = false

export function sink(_flowId: string, _nodeId: string, config: Config): Sink {
  return { kind: 'action', targetId: config.action }
}

// ── Phase 6: app-registry metadata (D2) ─────────────────────────────────────

export const label = 'Action'
export const category = 'Destinations' as const
export const glyph = IconZap
// Orange — the mockup doesn't show an Action node explicitly; a warm hue
// distinct from the amber chrome accent (Deploy button, focus rings) so an
// action node's cap never reads as "the same color as the UI chrome".
export const accentToken = 'var(--color-node-orange)'
export const tint = 'var(--color-node-orange-tint)'
/** Terminal node — 0 outputs. */
export const outputs = 0

export const defaults: Config = {
  action: '',
}

/** UX-only — Go's SaveFlow validator is authoritative (the ref must resolve to a desktop actions.yml entry). */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!config.action || !config.action.trim()) errors.push('action is required')
  return errors
}
