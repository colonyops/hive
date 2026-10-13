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

type QuotedComment = LineComment & { context: string }

export type HighlightBox = { top: number; left: number; width: number; height: number }
export type ReviewHighlight = { id: string; boxes: HighlightBox[] }

/**
 * The range of quote's text inside blocks, matched with whitespace ignored:
 * a selection's text joins blocks and lines with newlines that the DOM's
 * text does not hold. Null when the quote is not there.
 */
export function quoteRange(blocks: readonly Element[], quote: string): Range | null {
  const target = quote.replace(/\s+/g, '')
  if (!target) return null
  let text = ''
  const positions: { node: Text; offset: number }[] = []
  for (const block of blocks) {
    const walker = block.ownerDocument.createTreeWalker(block, NodeFilter.SHOW_TEXT)
    for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
      const value = node.data
      for (let offset = 0; offset < value.length; offset++) {
        if (/\s/.test(value[offset])) continue
        text += value[offset]
        positions.push({ node, offset })
      }
    }
  }
  const at = text.indexOf(target)
  if (at < 0) return null
  const start = positions[at]
  const end = positions[at + target.length - 1]
  const range = blocks[0].ownerDocument.createRange()
  range.setStart(start.node, start.offset)
  range.setEnd(end.node, end.offset + 1)
  return range
}

// A range's client rects come one per inline fragment; a line reads as one
// box.
function lineBoxes(rects: readonly DOMRect[]): DOMRect[] {
  const lines: DOMRect[] = []
  for (const rect of rects) {
    if (rect.width === 0 || rect.height === 0) continue
    const line = lines.find(
      (other) => Math.abs(other.top + other.height / 2 - (rect.top + rect.height / 2)) < rect.height / 2,
    )
    if (!line) {
      lines.push(new DOMRect(rect.left, rect.top, rect.width, rect.height))
      continue
    }
    const left = Math.min(line.left, rect.left)
    const top = Math.min(line.top, rect.top)
    line.width = Math.max(line.right, rect.right) - left
    line.height = Math.max(line.bottom, rect.bottom) - top
    line.x = left
    line.y = top
  }
  return lines
}

/**
 * Where each comment draws, in layer's coordinates: a box per line of the
 * text it quotes, so a comment on one word marks that word. A quote the
 * blocks no longer hold, or a block with no text such as a diagram, draws
 * the comment's blocks whole.
 */
export function reviewHighlights(
  root: HTMLElement,
  layer: HTMLElement,
  comments: readonly QuotedComment[],
): ReviewHighlight[] {
  const base = layer.getBoundingClientRect()
  const highlights: ReviewHighlight[] = []
  for (const comment of comments) {
    const blocks = [...root.querySelectorAll<HTMLElement>(`[data-review-ids~="${CSS.escape(comment.id)}"]`)]
    if (!blocks.length) continue
    const range = quoteRange(blocks, comment.context)
    const rects = range ? lineBoxes([...range.getClientRects()]) : blocks.map((block) => block.getBoundingClientRect())
    highlights.push({
      id: comment.id,
      boxes: rects.map((rect) => ({
        top: rect.top - base.top,
        left: rect.left - base.left,
        width: rect.width,
        height: rect.height,
      })),
    })
  }
  return highlights
}

const area = (highlight: ReviewHighlight) => highlight.boxes.reduce((sum, box) => sum + box.width * box.height, 0)

/**
 * The comment at a point in layer's coordinates. Where highlights overlap the
 * smallest answers, or, when current is one of them, the next larger one, so
 * repeated clicks cycle through them.
 */
export function highlightAt(
  highlights: readonly ReviewHighlight[],
  x: number,
  y: number,
  current: string | null = null,
): string | null {
  const here = highlights
    .filter((highlight) =>
      highlight.boxes.some(
        (box) => x >= box.left && x <= box.left + box.width && y >= box.top && y <= box.top + box.height,
      ),
    )
    .sort((a, b) => area(a) - area(b))
  if (!here.length) return null
  const at = here.findIndex((highlight) => highlight.id === current)
  return at < 0 ? here[0].id : here[(at + 1) % here.length].id
}
