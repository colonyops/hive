// github-filter is a declarative 2-output processor (1 in / 2 out: port 0
// pass, port 1 fail). This file is the editor's half of the node: its Config
// shape, its palette metadata, and a live validity check.
//
// The rule itself lives in Go (internal/app/runtime/filter.go), which is what
// evaluates it. Its glob dialect is unusual — "[", "]", "?", "{" and "}" are
// literal characters — so a second implementation here would be a second
// place for that to drift.

export const type = 'github-filter'
export const role = 'processor' as const
// Fixed 2-port node: port 0 = pass, port 1 = fail (leave port 1 unwired for
// today's plain "drop" filter behavior).
export const outputs = 2

export interface Config {
  repos?: string[]
  exclude_repos?: string[]
  authors?: string[]
  exclude_authors?: string[]
  labels?: string[]
  exclude_labels?: string[]
  types?: string[]
  reasons?: string[]
  ci?: string[]
  review?: string[]
}

// ── App-registry metadata ───────────────────────────────────────────────────

export const label = 'GitHub filter'
export const category = 'Process' as const
// Teal — matches the mockup's Filter node cap (8c status states).
export const accentToken = 'var(--color-node-teal)'
export const tint = 'var(--color-node-teal-tint)'

export const defaults: Config = {}

/**
 * UX-only — an empty filter matches every message (the fail port never
 * fires), which is almost always an authoring mistake rather than intent,
 * so the drawer flags it before Deploy. The backend validator enforces the
 * same rule when it saves the flow.
 */
export function validate(config: Config): string[] {
  const groups: Array<string[] | undefined> = [
    config.repos,
    config.exclude_repos,
    config.authors,
    config.exclude_authors,
    config.labels,
    config.exclude_labels,
    config.types,
    config.reasons,
    config.ci,
    config.review,
  ]
  const hasAny = groups.some((group) => group && group.length > 0)
  return hasAny ? [] : ['at least one filter group must be set']
}
