// Hand-rolled serializer for the feed editor's YAML preview: renders one
// feed entry the way the backend writes it into profiles.yaml (block style,
// 2-space indent, filter groups omitted when empty). Preview-only — the
// backend owns the real YAML surgery, so no yaml dependency is warranted
// for a shape this small.

import type { FeedDef, FilterDef } from '../types/feed'

// Plain-safe scalars: start and end on a word-ish character, no YAML
// indicator characters anywhere (interior spaces are fine — "Team PRs" stays
// unquoted like the backend writes it). Anything else — globs with
// *, ?, [, {, }, commas, leading/trailing spaces — is double-quoted;
// JSON string escaping is valid YAML double-quote escaping.
const PLAIN_SCALAR = /^[A-Za-z0-9_](?:[A-Za-z0-9 _.@/-]*[A-Za-z0-9_.@/-])?$/
// Strings YAML would reparse as another type must be quoted to stay strings.
const AMBIGUOUS = /^(?:true|false|null|yes|no|on|off|[+-]?\d+(?:\.\d+)?)$/i

function scalar(value: string): string {
  return PLAIN_SCALAR.test(value) && !AMBIGUOUS.test(value) ? value : JSON.stringify(value)
}

function listLines(key: string, values: string[] | null | undefined): string[] {
  if (!values || values.length === 0) return []
  return [`${key}:`, ...values.map((value) => `  - ${scalar(value)}`)]
}

// filterGroups fixes the render order to match the backend struct order.
const filterGroups: Array<keyof FilterDef> = [
  'repos', 'exclude_repos', 'authors', 'exclude_authors', 'labels', 'exclude_labels', 'types', 'reasons',
]

/**
 * Render a feed as a `profiles[].feeds` sequence entry. An empty id is
 * omitted entirely: on create the backend derives it from the name.
 */
export function feedEntryYaml(def: FeedDef): string {
  const lines: string[] = []
  if (def.id) lines.push(`id: ${scalar(def.id)}`)
  lines.push(`name: ${scalar(def.name)}`)
  const sources = def.sources ?? []
  if (sources.length === 0) {
    lines.push('sources: []')
  } else {
    lines.push('sources:', ...sources.map((id) => `  - ${scalar(id)}`))
  }
  const filters = def.filters ?? {}
  const filterLines = filterGroups.flatMap((key) => listLines(key, filters[key]).map((line) => `  ${line}`))
  if (filterLines.length > 0) lines.push('filters:', ...filterLines)
  return lines.map((line, i) => (i === 0 ? `- ${line}` : `  ${line}`)).join('\n')
}
