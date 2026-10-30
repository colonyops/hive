import { occurrencesIn, type DiffSearchMatch, type DiffSide } from './diffReview'

export const SEARCH_HIGHLIGHT = 'hive-diff-match'
export const SEARCH_HIGHLIGHT_ACTIVE = 'hive-diff-match-active'

/** The ::highlight rules, which must sit inside each diff's shadow root to reach its text. */
export const SEARCH_HIGHLIGHT_CSS = `
  ::highlight(${SEARCH_HIGHLIGHT}) { background-color: color-mix(in srgb, var(--color-severity-warning) 40%, transparent); }
  ::highlight(${SEARCH_HIGHLIGHT_ACTIVE}) { background-color: var(--color-severity-warning); color: var(--color-app); }
`

function lineSide(line: Element): DiffSide {
  const type = line.getAttribute('data-line-type')
  if (type === 'change-deletion') return 'deletions'
  if (type === 'change-addition') return 'additions'
  return line.closest('[data-deletions]') ? 'deletions' : 'additions'
}

function rangeInLine(line: Element, start: number, end: number): Range | null {
  const walker = document.createTreeWalker(line, NodeFilter.SHOW_TEXT)
  const range = document.createRange()
  let offset = 0
  let started = false
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const length = node.textContent?.length ?? 0
    if (!started && start < offset + length) {
      range.setStart(node, start - offset)
      started = true
    }
    if (started && end <= offset + length) {
      range.setEnd(node, end - offset)
      return range
    }
    offset += length
  }
  return null
}

/**
 * Marks the query in the lines CodeView has mounted, through the CSS Custom
 * Highlight API. Wrapping hits in elements would fight the library, which owns
 * and replaces these rows as they scroll.
 */
export function paintSearch(
  mounted: readonly { path: string; root: ParentNode }[],
  query: string,
  active: DiffSearchMatch | null,
): void {
  if (typeof CSS === 'undefined' || !('highlights' in CSS)) return
  const needle = query.trim().toLowerCase()
  const all: Range[] = []
  const current: Range[] = []
  if (needle) {
    for (const { path, root } of mounted) {
      for (const line of root.querySelectorAll('[data-line]')) {
        const hits = occurrencesIn(line.textContent ?? '', needle)
        if (!hits.length) continue
        const number = Number(line.getAttribute('data-line'))
        const side = lineSide(line)
        hits.forEach((at, occurrence) => {
          const range = rangeInLine(line, at, at + needle.length)
          if (!range) return
          const isActive =
            active?.path === path && active.side === side && active.line === number && active.occurrence === occurrence
          ;(isActive ? current : all).push(range)
        })
      }
    }
  }
  CSS.highlights.set(SEARCH_HIGHLIGHT, new Highlight(...all))
  CSS.highlights.set(SEARCH_HIGHLIGHT_ACTIVE, new Highlight(...current))
}

export function clearSearchPaint(): void {
  if (typeof CSS === 'undefined' || !('highlights' in CSS)) return
  CSS.highlights.delete(SEARCH_HIGHLIGHT)
  CSS.highlights.delete(SEARCH_HIGHLIGHT_ACTIVE)
}
