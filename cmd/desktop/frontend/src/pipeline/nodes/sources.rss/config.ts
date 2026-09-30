// Runs on the backend (internal/app/sources/rss); role 'source' means no runtime.ts here.

import IconRss from '~icons/lucide/rss'
import { intervalError } from '../../lib/sourceInterval'

export const type = 'sources.rss'
export const role = 'source' as const
export const sourceKind = 'rss'

/** Mirrors Go's defaultLimit and maxLimit (internal/app/sources/rss/config.go). */
export const DEFAULT_LIMIT = 50
export const MAX_LIMIT = 500

export interface Config {
  /** The RSS, Atom or JSON Feed document to fetch. */
  url: string
  /** How many of the feed's newest entries to ingest per fetch. */
  limit?: number
  /** Go duration string: the shortest time between fetches. */
  interval?: string
  icon?: string
  /** Content hash of an uploaded image shown as the feed mark instead of `icon`. */
  image?: string
}

export const label = 'RSS feed'
export const category = 'Sources' as const
// A feed is a protocol rather than a product, so it wears the generic source
// hue and the glyph the icon set already names for it.
export const glyph = IconRss
export const accentToken = 'var(--color-node-blue)'
export const tint = 'var(--color-node-blue-tint)'

// An interval by default: the poll tick is far more often than any feed
// publishes, and a node added without one polls a stranger's server every tick.
export const defaults: Config = {
  url: '',
  interval: '30m',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []

  const url = (config.url ?? '').trim()
  if (!url) errors.push('a feed URL is required')
  else if (!/^https?:\/\/./i.test(url)) errors.push('the feed URL must start with http:// or https://')
  else if (/^https?:\/\/[^/@]*@/i.test(url)) errors.push('the feed URL must not contain a username or password')

  const limit = config.limit
  if (limit !== undefined && (!Number.isInteger(limit) || limit < 1 || limit > MAX_LIMIT)) {
    errors.push(`limit must be a whole number from 1 to ${MAX_LIMIT}`)
  }

  const interval = intervalError(config.interval)
  if (interval) errors.push(interval)

  if (config.image && !/^[0-9a-f]{32}$/.test(config.image)) errors.push('image is not a valid mark reference')
  return errors
}
