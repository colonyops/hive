// Runs on the backend (internal/app/sources/gitea); role 'source' means no runtime.ts here.

import GiteaMark from '../../../components/marks/GiteaMark.vue'
import { intervalError } from '../../lib/sourceInterval'

export const type = 'sources.gitea'
export const role = 'source' as const
// The inbox item sourceKind this node's items carry — the single source of
// truth lib/itemPresentation.ts's sourceKindForNodeType derives from.
export const sourceKind = 'gitea'

export const KINDS = ['search', 'notifications'] as const
export const ITEMS = ['all', 'issues', 'pulls'] as const
export const STATES = ['open', 'closed', 'all'] as const
export const INVOLVING = ['created', 'assigned', 'mentioned', 'review_requested', 'reviewed'] as const

export type Kind = (typeof KINDS)[number]
export type Items = (typeof ITEMS)[number]
export type State = (typeof STATES)[number]
export type Involving = (typeof INVOLVING)[number]

export const MAX_SEARCH_LIMIT = 100
export const MAX_NOTIFICATIONS_LIMIT = 50

export interface Config {
  /**
   * A ref and never a token: flows/ is dotfiles-managed, so an embedded token
   * would be a token in a git repo. The account half names the host, because
   * that is bound to the credential when the instance is connected.
   */
  credential: string
  /** "search" runs a filtered query; "notifications" drains the inbox. */
  kind: Kind
  /** Which items a search returns. Unused for "notifications". */
  items?: Items
  /** Which lifecycle states a search returns. Unused for "notifications". */
  state?: State
  /** Relationships to the connected account; several are a union. */
  involving?: Involving[]
  /** Limit a search to one user's or organization's repositories. */
  owner?: string
  /** Return items carrying any of these labels. */
  labels?: string[]
  /** Free-text search over title and body. */
  text?: string
  /** Max items per fetch (search caps at 100, notifications at 50). */
  limit?: number
  /** Go duration string: the shortest time between fetches. */
  interval?: string
}

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'Gitea source'
export const category = 'Sources' as const
// Gitea's own mark, the same one the Integrations screen and the inbox source
// badge use — a source node stands for a product, so it wears that product's
// identity rather than a lucide approximation of it.
export const glyph = GiteaMark
/** A product logomark, not a lucide glyph — see NodeTypeDefinition.logoMark. */
export const logoMark = true
export const accentToken = 'var(--color-brand-gitea)'
export const tint = 'var(--color-brand-gitea-tint)'

export const defaults: Config = {
  credential: '',
  kind: 'search',
  items: 'all',
  state: 'open',
  involving: [],
}

/** The search-only fields, so validation and the editor agree on the set. */
const SEARCH_ONLY: Array<keyof Config> = ['items', 'state', 'involving', 'owner', 'labels', 'text']

function isSet(value: Config[keyof Config]): boolean {
  if (Array.isArray(value)) return value.length > 0
  return typeof value === 'string' ? value.trim().length > 0 : value !== undefined
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  const credential = (config.credential ?? '').trim()
  if (!credential) {
    errors.push('a source needs a connected Gitea account')
  } else if (!/^gitea\/[^/]+$/.test(credential)) {
    errors.push('credential must look like "gitea/<host>-<login>"')
  }

  if (config.kind === 'search') {
    if (config.items && !ITEMS.includes(config.items)) {
      errors.push(`items must be one of ${ITEMS.join(', ')}`)
    }
    if (config.state && !STATES.includes(config.state)) {
      errors.push(`state must be one of ${STATES.join(', ')}`)
    }
    for (const involving of config.involving ?? []) {
      if (!INVOLVING.includes(involving)) {
        errors.push(`involving must be one of ${INVOLVING.join(', ')}`)
        break
      }
    }
    // Gitea's labels parameter is comma-joined with no escaping, so a label
    // carrying one would silently split into two nonexistent filters.
    for (const label of config.labels ?? []) {
      if (label.includes(',')) {
        errors.push(`label "${label}" contains a comma, which Gitea's search cannot express`)
        break
      }
    }
    if ((config.limit ?? 0) > MAX_SEARCH_LIMIT) errors.push(`search limit caps at ${MAX_SEARCH_LIMIT}`)
  } else if (config.kind === 'notifications') {
    const set = SEARCH_ONLY.filter((field) => isSet(config[field]))
    if (set.length > 0) errors.push(`a notifications source takes no search filters (remove ${set.join(', ')})`)
    if ((config.limit ?? 0) > MAX_NOTIFICATIONS_LIMIT) errors.push(`notifications limit caps at ${MAX_NOTIFICATIONS_LIMIT}`)
  } else {
    errors.push('kind must be "search" or "notifications"')
  }

  if ((config.limit ?? 0) < 0) errors.push('limit must not be negative')
  const interval = intervalError(config.interval)
  if (interval) errors.push(interval)
  return errors
}
