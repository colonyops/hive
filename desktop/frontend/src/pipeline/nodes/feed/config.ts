// feed is a terminal node (1 in / 0 out): the arriving message creates an
// unread inbox-item membership claim. The node *is* the feed — its identity is
// the flow-qualified node id "<flowId>/<nodeId>", the durable claim key the
// sidebar reads back. role/sink here are the engine's single source
// of truth for "how a feed node becomes a commit output" — runGraph imports
// `sink`/`unread` directly rather than re-encoding the mapping.

import IconRss from '~icons/lucide/rss'
import type { Sink } from '../../types'
import { isFeedIcon } from '../../../lib/feedIcons'

export const type = 'feed'
export const role = 'output' as const

// A feed node's identity is still its node id; these fields are purely
// cosmetic sidebar presentation. `icon` picks the glyph shown in the tree
// (one of the scoped keys in lib/feedIcons), and `description` is the hover
// tooltip context (especially useful for LLM-generated feeds). Both optional;
// mirrors Go's flow.FeedConfig.
export interface Config {
  icon?: string
  description?: string
}

/** Longest description the editor accepts — mirrors Go's feedDescriptionMaxLen. */
export const descriptionMaxLen = 500

/** Newly observed inbox items land unread until the user reads them. */
export const unread = true

/** The feed's durable key is the flow-qualified node id. */
export function sink(flowId: string, nodeId: string): Sink {
  return { kind: 'feed', targetId: `${flowId}/${nodeId}` }
}

// ── App-registry metadata ───────────────────────────────────────────────────

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

/** Validates the feed's cosmetic fields, matching Go's FeedConfig.Validate. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (config.icon && !isFeedIcon(config.icon)) {
    errors.push(`Icon "${config.icon}" is not a supported feed icon.`)
  }
  if ((config.description?.length ?? 0) > descriptionMaxLen) {
    errors.push(`Description must be at most ${descriptionMaxLen} characters.`)
  }
  return errors
}
