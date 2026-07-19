// feed is a terminal node (1 in / 0 out): the arriving msg upserts into this
// feed as an unread feed_item. The node *is* the feed — its identity is the
// flow-qualified node id "<flowId>/<nodeId>", which is the durable feed_item
// key the sidebar reads back. role/sink here are the engine's single source
// of truth for "how a feed node becomes a commit output" — runGraph imports
// `sink`/`unread` directly rather than re-encoding the mapping.

import IconRss from '~icons/lucide/rss'
import type { Sink } from '../../types'

export const type = 'feed'
export const role = 'output' as const

// A feed node carries no config — the node id is its identity.
export type Config = Record<string, never>

/** New feed items land unread until the user reads them. */
export const unread = true

/** The feed's durable key is the flow-qualified node id. */
export function sink(flowId: string, nodeId: string): Sink {
  return { kind: 'feed', targetId: `${flowId}/${nodeId}` }
}

// ── Phase 6: app-registry metadata (D2) ─────────────────────────────────────

export const label = 'Feed'
export const category = 'Destinations' as const
export const glyph = IconRss
// Green — the mockup doesn't show a Feed node explicitly; reuses the
// existing --color-feeds hue (already this app's "feed" meaning color).
export const accentToken = 'var(--color-node-green)'
export const tint = 'var(--color-node-green-tint)'
/** Terminal node — 0 outputs. */
export const outputs = 0

export const defaults: Config = {}

/** A feed node has no config to validate. */
export function validate(_config: Config): string[] {
  return []
}
