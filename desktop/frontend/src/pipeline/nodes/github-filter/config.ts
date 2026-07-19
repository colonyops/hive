// github-filter is a declarative 2-output processor (1 in / 2 out: port 0
// pass, port 1 fail) — a faithful port of internal/desktop/feed/filters.go's
// FilterDef.matches(). This file holds the Config shape and the pure
// glob-matching helpers; runtime.ts wires them into the port-routing
// ProcessorRuntime.

import IconFilter from '~icons/lucide/filter'

export const type = 'github-filter'
export const role = 'processor' as const
// isolate:false — trusted, declarative, no author JS — the engine hosts
// this in the shared worker alongside every other isolate:false instance.
export const isolate = false
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
}

// ── Phase 6: app-registry metadata (D2) ─────────────────────────────────────

export const label = 'GitHub filter'
export const category = 'Process' as const
export const glyph = IconFilter
// Teal — matches the mockup's Filter node cap (8c status states).
export const accentToken = 'var(--color-node-teal)'
export const tint = 'var(--color-node-teal-tint)'

export const defaults: Config = {}

/**
 * UX-only — an empty filter matches every message (the fail port never
 * fires), which is almost always an authoring mistake rather than intent,
 * so the drawer flags it. Go's SaveFlow validator does not reject this
 * (D1 lists "empty github-filter" as a hard error there, in fact — this
 * mirrors that rule for live feedback before Deploy).
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
  ]
  const hasAny = groups.some((group) => group && group.length > 0)
  return hasAny ? [] : ['at least one filter group must be set']
}

/**
 * Minimal doublestar-equivalent glob matcher — no npm dependency. Supports
 * exactly what filters.go's doublestar usage needs:
 *   - `*`  matches any run of characters EXCEPT `/` (one path segment)
 *   - `**` matches any run of characters INCLUDING `/` (zero or more
 *     segments)
 * Every other character is matched literally, INCLUDING `[` and `]` — this
 * matcher has no bracket character-class support at all, so a pattern like
 * "*[bot]" already treats "[bot]" as four literal characters with no extra
 * escaping step, which is exactly the behavior filters.go achieves in Go by
 * explicitly escaping "[" / "]" before calling into doublestar (that
 * escaping has no counterpart to write here — literal is this matcher's
 * only mode for those characters).
 */
export function globToRegExp(pattern: string): RegExp {
  let source = '^'
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i]
    if (c === '*') {
      if (pattern[i + 1] === '*') {
        source += '.*'
        i++ // consume the second '*'
      } else {
        source += '[^/]*'
      }
    } else if ('\\^$.|?+()[]{}'.includes(c)) {
      source += '\\' + c
    } else {
      source += c
    }
  }
  return new RegExp(source + '$')
}

export function matchGlob(pattern: string, value: string): boolean {
  return globToRegExp(pattern).test(value)
}

function matchAnyGlob(patterns: string[] | undefined, value: string): boolean {
  if (!patterns) return false
  return patterns.some((pattern) => matchGlob(pattern, value))
}

// Authors match case-insensitively (Go: strings.ToLower on both sides).
function matchAnyAuthorGlob(patterns: string[] | undefined, author: string): boolean {
  if (!patterns) return false
  const lowerAuthor = author.toLowerCase()
  return patterns.some((pattern) => matchGlob(pattern.toLowerCase(), lowerAuthor))
}

function matchAnyLabel(patterns: string[] | undefined, labels: string[] | undefined): boolean {
  if (!patterns || !labels) return false
  return labels.some((label) => matchAnyGlob(patterns, label))
}

function containsFold(values: string[] | undefined, value: string): boolean {
  if (!values) return false
  const lower = value.toLowerCase()
  return values.some((v) => v.toLowerCase() === lower)
}

/**
 * The GitHub item shape the filter inspects, read off msg.Payload. Mirrors
 * internal/desktop/feed.Item's JSON tags (id/kind/repo/author/reason/labels
 * lowercase) — the same shape internal/desktop/pipeline/github_source.go
 * encodes as a Msg's Payload.
 */
export interface FilterableItem {
  repo?: string
  author?: string
  labels?: string[]
  kind?: string
  reason?: string
}

/**
 * matches ports internal/desktop/feed/filters.go's FilterDef.matches() rule
 * for rule: groups AND together; values within a group OR; exclude groups
 * win over includes. A missing reason (item.reason == "" / undefined)
 * matches no reasons filter — a reasons filter deliberately excludes
 * search-only items, per the Go implementation's comment.
 */
export function matches(config: Config, item: FilterableItem): boolean {
  const repo = item.repo ?? ''
  const author = item.author ?? ''
  const kind = item.kind ?? ''
  const reason = item.reason ?? ''

  if (matchAnyGlob(config.exclude_repos, repo)) return false
  if (config.repos && config.repos.length > 0 && !matchAnyGlob(config.repos, repo)) return false

  if (matchAnyAuthorGlob(config.exclude_authors, author)) return false
  if (config.authors && config.authors.length > 0 && !matchAnyAuthorGlob(config.authors, author)) return false

  if (matchAnyLabel(config.exclude_labels, item.labels)) return false
  if (config.labels && config.labels.length > 0 && !matchAnyLabel(config.labels, item.labels)) return false

  if (config.types && config.types.length > 0 && !containsFold(config.types, kind)) return false

  if (config.reasons && config.reasons.length > 0 && !containsFold(config.reasons, reason)) return false

  return true
}
