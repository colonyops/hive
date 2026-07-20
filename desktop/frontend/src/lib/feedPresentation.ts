// Presentation helpers for the inbox feed (9a): which source an item came
// from, how its type reads, and a one-line snippet of its body. The feed is a
// multi-source inbox — today only GitHub is wired, but the source is derived
// here so Grafana/Slack/PostHog slot in one place.

export interface FeedSource {
  key: 'github'
  label: string
}

// The originating source of a feed item. Only GitHub produces items today; the
// signature takes the item so future sources can branch on its fields here
// rather than at every call site.
export function feedSource(item?: { url?: string }): FeedSource {
  void item
  return { key: 'github', label: 'GitHub' }
}

// Human label for an item's type, shown in the row's type pill and the detail
// header. Falls back to the raw kind for anything unmapped.
export function typeLabel(kind: string): string {
  if (kind === 'PR') return 'Pull Request'
  if (kind === 'Issue') return 'Issue'
  return kind || 'Item'
}

// A one-line, plain-text preview of a markdown body for the inbox row — the
// first non-empty line with the common markdown markers stripped.
export function bodySnippet(body: string): string {
  for (const raw of body.split('\n')) {
    const line = raw
      .replace(/^#{1,6}\s+/, '') // headings
      .replace(/^\s*[-*+]\s+\[[ xX]\]\s+/, '') // task-list markers
      .replace(/^\s*[-*+>]\s+/, '') // list / blockquote markers
      .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1') // links / images → text
      .replace(/[*_`~]/g, '') // inline emphasis / code ticks
      .trim()
    if (line) return line
  }
  return ''
}
