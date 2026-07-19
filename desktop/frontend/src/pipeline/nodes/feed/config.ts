// feed is a terminal node (1 in / 0 out): the arriving msg upserts into the
// referenced feed as an unread feed_item. role/sink here are the engine's
// single source of truth for "how a feed node becomes a commit output" —
// runGraph imports `sink`/`unread` directly rather than re-encoding the
// mapping. Phase 6 additionally uses `type`/`role` for the palette registry
// and a (future) editor.vue for the `feed` field.

import type { Sink } from '../../types'

export const type = 'feed'
export const role = 'output' as const

export interface Config {
  /** feed id in profiles/*.yml — a durable feed_item key. */
  feed: string
}

/** New feed items land unread until the user reads them. */
export const unread = true

export function sink(config: Config): Sink {
  return { kind: 'feed', targetId: config.feed }
}
