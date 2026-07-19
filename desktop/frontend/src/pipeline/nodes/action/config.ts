// action is a terminal node (1 in / 0 out): the arriving msg enqueues an
// output_command against the referenced .hive/actions.yml action id. See
// nodes/feed/config.ts for the parallel terminal (sink/unread are the
// engine's single source of truth for commit-tagging a terminal node).

import type { Sink } from '../../types'

export const type = 'action'
export const role = 'output' as const

export interface Config {
  /** action id in .hive/actions.yml (Phase 5). */
  action: string
}

/** Action outputs are enqueued commands, not feed items — unread has no meaning here. */
export const unread = false

export function sink(config: Config): Sink {
  return { kind: 'action', targetId: config.action }
}
