// github-source is a source node (0 in / 1 out): it embeds its own GitHub
// fetch config — a "search" source runs a query, a "notifications" source
// drains the authenticated user's inbox. The source itself runs on the
// backend (internal/desktop/pipeline.Source / githubSource) — the frontend
// never executes it, only consumes the msgs it already appended to the log,
// so there is no runtime.ts here (role: 'source' means "backend-run" per D2).
//
// The engine still needs to know which log topic feeds this node: the backend
// producer appends a source node's items under topic "source:<flowId>/<nodeId>"
// (see github_source.go), so an entry github-source node only accepts messages
// on that flow-qualified topic — see engine/runGraph.ts's `acceptsEntry`.

import IconGithub from '~icons/lucide/github'

export const type = 'github-source'
export const role = 'source' as const

export type SourceKind = 'search' | 'notifications'

export interface Config {
  /** "search" runs a GitHub search query; "notifications" drains the inbox. */
  kind: SourceKind
  /** Search query (required for kind "search"; unused for "notifications"). */
  query?: string
  /** Max items per fetch (search caps at 100, notifications at 50). */
  limit?: number
}

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'GitHub source'
export const category = 'Sources' as const
export const glyph = IconGithub
// Blue — matches the mockup's GitHub source node cap (8c wiring/anatomy).
export const accentToken = 'var(--color-node-blue)'
export const tint = 'var(--color-node-blue-tint)'

export const defaults: Config = {
  kind: 'search',
  query: '',
}

/** UX-only — Go's SaveFlow validator is authoritative. */
export function validate(config: Config): string[] {
  const errors: string[] = []
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
  return errors
}
