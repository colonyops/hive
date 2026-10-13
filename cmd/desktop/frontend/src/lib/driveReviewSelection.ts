/** A selection in a rendered document, as the source lines it covers. */
export type SourceRange = { startLine: number; endLine: number; context: string }

function sourceBlock(node: Node, root: HTMLElement): HTMLElement | null {
  const element = node instanceof HTMLElement ? node : node.parentElement
  const block = element?.closest<HTMLElement>('[data-source-start]') ?? null
  return block && root.contains(block) ? block : null
}

function lineAttr(element: HTMLElement, name: string): number | null {
  const value = Number(element.getAttribute(name))
  return Number.isInteger(value) && value > 0 ? value : null
}

/**
 * Maps a selection inside root to the source lines of the blocks at its two
 * ends. It is null when the selection is collapsed, leaves root, or ends
 * outside any block that carries source lines.
 */
export function selectionToRange(sel: Selection, root: HTMLElement): SourceRange | null {
  if (sel.isCollapsed || sel.rangeCount === 0) return null
  const range = sel.getRangeAt(0)
  if (!root.contains(range.startContainer) || !root.contains(range.endContainer)) return null
  const first = sourceBlock(range.startContainer, root)
  const last = sourceBlock(range.endContainer, root)
  if (!first || !last) return null

  const starts = [lineAttr(first, 'data-source-start'), lineAttr(last, 'data-source-start')]
  const ends = [lineAttr(first, 'data-source-end'), lineAttr(last, 'data-source-end')]
  if (starts.some((line) => line === null) || ends.some((line) => line === null)) return null
  const context = sel.toString().trim()
  if (!context) return null
  return {
    startLine: Math.min(...(starts as number[])),
    endLine: Math.max(...(ends as number[])),
    context,
  }
}

const commentClass = 'has-review-comment'
const activeClass = 'is-active-review'
const hoverClass = 'is-hovered-review'

/**
 * Marks the blocks under root that each comment covers: the innermost blocks
 * whose source lines overlap it get the has-review-comment class and list the
 * comment's id in data-review-ids. The blocks of the active comment also get
 * is-active-review, and those of the hovered one is-hovered-review. Earlier
 * marks are removed first.
 */
export function markReviewedBlocks(
  root: HTMLElement,
  comments: readonly { id: string; startLine: number; endLine: number }[],
  active: string | null = null,
  hovered: string | null = null,
): void {
  for (const marked of root.querySelectorAll<HTMLElement>(`.${commentClass}`)) {
    marked.classList.remove(commentClass, activeClass, hoverClass)
    marked.removeAttribute('data-review-ids')
  }
  if (!comments.length) return

  const blocks = [...root.querySelectorAll<HTMLElement>('[data-source-start]')].map((element) => ({
    element,
    start: Number(element.dataset.sourceStart),
    end: Number(element.dataset.sourceEnd),
  }))
  for (const comment of comments) {
    const overlapping = blocks.filter((block) => block.start <= comment.endLine && block.end >= comment.startLine)
    const innermost = overlapping.filter(
      (block) => !overlapping.some((other) => other !== block && block.element.contains(other.element)),
    )
    for (const { element } of innermost) {
      element.classList.add(commentClass)
      if (comment.id === active) element.classList.add(activeClass)
      if (comment.id === hovered) element.classList.add(hoverClass)
      element.dataset.reviewIds = [element.dataset.reviewIds, comment.id].filter(Boolean).join(' ')
    }
  }
}
