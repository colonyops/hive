import type { SelectedLineRange } from '@pierre/diffs'

function lineOf(node: Node): HTMLElement | null {
  const element = node instanceof HTMLElement ? node : node.parentElement
  return element?.closest<HTMLElement>('[data-line]') ?? null
}

function sideOf(line: HTMLElement): 'additions' | 'deletions' {
  const type = line.getAttribute('data-line-type')
  if (type === 'change-deletion') return 'deletions'
  if (type === 'change-addition') return 'additions'
  return line.closest('[data-deletions]') ? 'deletions' : 'additions'
}

/** Lines the user picked to comment on. */
export interface DiffLineSelection {
  path: string
  range: SelectedLineRange
  /** The exact text, when the selection is code on one line rather than whole rows. */
  text?: string
  rect: { top: number; left: number; width: number; height: number }
}

/**
 * The text selected across one file's diff lines, or null. The diff sits in shadow
 * roots, where WebKit's Selection reports itself collapsed whatever is selected, so
 * only getComposedRanges can see where the selection really is.
 */
export function diffTextSelection(
  selection: Selection | null,
  mounted: readonly { path: string; root: ShadowRoot }[],
): DiffLineSelection | null {
  const [composed] = selection?.getComposedRanges({ shadowRoots: mounted.map(({ root }) => root) }) ?? []
  if (!composed || composed.collapsed) return null
  const startLine = lineOf(composed.startContainer)
  const endLine = lineOf(composed.endContainer)
  if (!startLine || !endLine) return null
  const owner = mounted.find(({ root }) => root.contains(startLine))
  if (!owner || !owner.root.contains(endLine)) return null

  const range = document.createRange()
  range.setStart(composed.startContainer, composed.startOffset)
  range.setEnd(composed.endContainer, composed.endOffset)
  const selected = range.toString()
  if (!selected.trim()) return null
  // Across lines the browser's text runs them together, as the rows are not text
  // nodes of one another. The diff's own rows quote those properly.
  const text = startLine === endLine ? selected : undefined
  const { top, left, width, height } = range.getBoundingClientRect()
  return {
    path: owner.path,
    range: {
      start: Number(startLine.getAttribute('data-line')),
      side: sideOf(startLine),
      end: Number(endLine.getAttribute('data-line')),
      endSide: sideOf(endLine),
    },
    text,
    rect: { top, left, width, height },
  }
}
