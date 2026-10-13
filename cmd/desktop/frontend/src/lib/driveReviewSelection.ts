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

type LineComment = { id: string; startLine: number; endLine: number }

/**
 * Marks the blocks under root that each comment covers: the innermost blocks
 * whose source lines overlap it get the has-review-comment class and list the
 * comment's id in data-review-ids. Earlier marks are removed first.
 */
export function markReviewedBlocks(root: HTMLElement, comments: readonly LineComment[]): void {
  for (const marked of root.querySelectorAll<HTMLElement>(`.${commentClass}`)) {
    marked.classList.remove(commentClass)
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
      element.dataset.reviewIds = [element.dataset.reviewIds, comment.id].filter(Boolean).join(' ')
    }
  }
}

const span = (comment: LineComment) => comment.endLine - comment.startLine

/**
 * How many comments enclose each one, so a nested comment draws inside the one
 * around it. Of two comments on the same lines, the later one nests.
 */
export function commentDepths(comments: readonly LineComment[]): Map<string, number> {
  const depths = new Map<string, number>()
  comments.forEach((comment, index) => {
    let depth = 0
    comments.forEach((other, otherIndex) => {
      if (other === comment || other.startLine > comment.startLine || other.endLine < comment.endLine) return
      if (span(other) > span(comment) || otherIndex < index) depth++
    })
    depths.set(comment.id, depth)
  })
  return depths
}

/**
 * The comment a pointer on a block with these ids means: the innermost, or,
 * when current is one of them, the next one out, so repeated clicks cycle
 * through comments that overlap.
 */
export function commentAtBlock(
  ids: readonly string[],
  comments: readonly LineComment[],
  current: string | null = null,
): string | null {
  const here = comments.filter((comment) => ids.includes(comment.id)).sort((a, b) => span(a) - span(b))
  if (!here.length) return null
  const at = here.findIndex((comment) => comment.id === current)
  return at < 0 ? here[0].id : here[(at + 1) % here.length].id
}

export type ReviewHighlight = { id: string; top: number; left: number; width: number; height: number; depth: number }

/**
 * One box per comment around every block it covers, in layer's coordinates,
 * so a comment across several blocks reads as one highlight.
 */
export function reviewHighlights(
  root: HTMLElement,
  layer: HTMLElement,
  comments: readonly LineComment[],
): ReviewHighlight[] {
  const base = layer.getBoundingClientRect()
  const depths = commentDepths(comments)
  const highlights: ReviewHighlight[] = []
  for (const comment of comments) {
    const rects = [...root.querySelectorAll<HTMLElement>(`[data-review-ids~="${CSS.escape(comment.id)}"]`)].map(
      (element) => element.getBoundingClientRect(),
    )
    if (!rects.length) continue
    const top = Math.min(...rects.map((rect) => rect.top))
    const left = Math.min(...rects.map((rect) => rect.left))
    const bottom = Math.max(...rects.map((rect) => rect.bottom))
    const right = Math.max(...rects.map((rect) => rect.right))
    highlights.push({
      id: comment.id,
      top: top - base.top,
      left: left - base.left,
      width: right - left,
      height: bottom - top,
      depth: depths.get(comment.id) ?? 0,
    })
  }
  return highlights
}
