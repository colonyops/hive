// sources.github is a source node (0 in / 1 out): it embeds its own GitHub
// fetch config — a "search" source runs a query, a "notifications" source
// drains the authenticated user's inbox. The source itself runs on the
// backend (ingest.Source / sources/github.githubSource) — the frontend
// never executes it, only consumes the msgs it already appended to the log,
// so there is no runtime.ts here (role: 'source' means "backend-run" per D2).
//
// The engine still needs to know which log topic feeds this node: the backend
// producer appends a source node's items under topic "source:<flowId>/<nodeId>"
// (see github_source.go), so an entry sources.github node only accepts messages
// on that flow-qualified topic — see engine/runGraph.ts's `acceptsEntry`.

import GithubMark from '../../../components/marks/GithubMark.vue'
import { intervalError } from '../../lib/sourceInterval'

export const type = 'sources.github'
export const role = 'source' as const
// The inbox item sourceKind this node's items carry — the single source of
// truth lib/itemPresentation.ts's sourceKindForNodeType derives from.
export const sourceKind = 'github'

export type SourceKind = 'search' | 'notifications'

export interface Config {
  /**
   * The connected account this source fetches as, as "github/<login>". A ref
   * and never a token: flows/ is dotfiles-managed, so an embedded token would
   * be a token in a git repo.
   */
  credential: string
  /** "search" runs a GitHub search query; "notifications" drains the inbox. */
  kind: SourceKind
  /** Search query (required for kind "search"; unused for "notifications"). */
  query?: string
  /** Max items per fetch (search caps at 100, notifications at 50). */
  limit?: number
  /** Go duration string: the shortest time between fetches. */
  interval?: string
}

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'GitHub source'
export const category = 'Sources' as const
// GitHub's own mark, the same one the Integrations screen and the inbox
// source badge use — a source node stands for a product, so it wears that
// product's identity rather than a lucide approximation of it.
export const glyph = GithubMark
/** A product logomark, not a lucide glyph — see NodeTypeDefinition.logoMark. */
export const logoMark = true
export const accentToken = 'var(--color-brand-github)'
export const tint = 'var(--color-brand-github-tint)'

export const defaults: Config = {
  credential: '',
  kind: 'search',
  query: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
  if (!config.credential || !config.credential.trim()) {
    errors.push('a source needs a connected GitHub account')
  } else if (!/^github\/[^/]+$/.test(config.credential.trim())) {
    errors.push('credential must look like "github/<login>"')
  }
  if (config.kind === 'search') {
    if (!config.query || !config.query.trim()) errors.push('a search source requires a query')
    if (typeof config.limit === 'number' && config.limit > 100) errors.push('search limit caps at 100')
  } else if (config.kind === 'notifications') {
    if (config.query && config.query.trim()) errors.push('a notifications source takes no query')
    if (typeof config.limit === 'number' && config.limit > 50) errors.push('notifications limit caps at 50')
  } else {
    errors.push('kind must be "search" or "notifications"')
  }
  if (typeof config.limit === 'number' && config.limit < 0) errors.push('limit must not be negative')
  const interval = intervalError(config.interval)
  if (interval) errors.push(interval)
  return errors
}
