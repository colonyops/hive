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
 * Maps a selection inside root to the source lines it covers. An end block
 * whose rendering keeps its line breaks narrows to the selected line; any
 * other block counts whole. It is null when the selection is collapsed,
 * leaves root, or ends outside any block that carries source lines.
 */
export function selectionToRange(sel: Selection, root: HTMLElement): SourceRange | null {
  if (sel.isCollapsed || sel.rangeCount === 0) return null
  const range = sel.getRangeAt(0)
  if (!root.contains(range.startContainer) || !root.contains(range.endContainer)) return null
  const first = sourceBlock(range.startContainer, root)
  const last = sourceBlock(range.endContainer, root)
  if (!first || !last) return null

  const firstStart = lineAttr(first, 'data-source-start')
  const firstEnd = lineAttr(first, 'data-source-end')
  const lastStart = lineAttr(last, 'data-source-start')
  const lastEnd = lineAttr(last, 'data-source-end')
  if (firstStart === null || firstEnd === null || lastStart === null || lastEnd === null) return null
  const context = sel.toString().trim()
  if (!context) return null
  return {
    startLine: lineBreaks(first) ? lineAt(first, range.startContainer, range.startOffset) : firstStart,
    endLine: lineBreaks(last) ? lineAt(last, range.endContainer, range.endOffset) : lastEnd,
    context,
  }
}

// Only a block whose rendering keeps its source line breaks can be narrowed;
// any other block, a diagram for one, keeps its whole range.
function lineBreaks(block: HTMLElement): boolean {
  return !!block.querySelector('br, pre') || !!block.closest('pre')
}

/**
 * How many source lines of block come before a point in it, so a selection
 * inside a multi-line block narrows to its own lines. A paragraph renders each
 * source line break as <br>; code keeps its newlines as text.
 */
function linesBefore(block: HTMLElement, node: Node, offset: number): number {
  const before = block.ownerDocument.createRange()
  before.setStart(block, 0)
  before.setEnd(node, offset)
  if (block.querySelector('pre') || block.closest('pre')) return (before.toString().match(/\n/g) ?? []).length
  return before.cloneContents().querySelectorAll('br').length
}

// A fenced block's range starts on its opening fence, so its code starts on
// the line the renderer names in data-source-code-start.
function lineAt(block: HTMLElement, node: Node, offset: number): number {
  const base = lineAttr(block, 'data-source-code-start') ?? lineAttr(block, 'data-source-start') ?? 0
  return Math.min(lineAttr(block, 'data-source-end') ?? base, base + linesBefore(block, node, offset))
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
 * text does not hold. Where the quote repeats, the first match that starts on
 * startLine wins, else the first match. Null when the quote is not there.
 */
export function quoteRange(blocks: readonly HTMLElement[], quote: string, startLine?: number): Range | null {
  const target = quote.replace(/\s+/g, '')
  if (!target) return null
  let text = ''
  const positions: { block: HTMLElement; node: Text; offset: number }[] = []
  for (const block of blocks) {
    const walker = block.ownerDocument.createTreeWalker(block, NodeFilter.SHOW_TEXT)
    for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
      const value = node.data
      for (let offset = 0; offset < value.length; offset++) {
        if (/\s/.test(value[offset])) continue
        text += value[offset]
        positions.push({ block, node, offset })
      }
    }
  }
  const at = matchOnLine(text, target, positions, startLine)
  if (at < 0) return null
  const start = positions[at]
  const end = positions[at + target.length - 1]
  const range = blocks[0].ownerDocument.createRange()
  range.setStart(start.node, start.offset)
  range.setEnd(end.node, end.offset + 1)
  return range
}

function matchOnLine(
  text: string,
  target: string,
  positions: readonly { block: HTMLElement; node: Text; offset: number }[],
  startLine: number | undefined,
): number {
  const first = text.indexOf(target)
  if (startLine === undefined) return first
  for (let at = first; at >= 0; at = text.indexOf(target, at + 1)) {
    const { block, node, offset } = positions[at]
    const line = lineBreaks(block) ? lineAt(block, node, offset) : lineAttr(block, 'data-source-start')
    if (line === startLine) return at
  }
  return first
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
    const range = quoteRange(blocks, comment.context, comment.startLine)
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

/** A live range's lines, in layer's coordinates. */
export function rangeBoxes(range: Range, layer: HTMLElement): HighlightBox[] {
  const base = layer.getBoundingClientRect()
  return lineBoxes([...range.getClientRects()]).map((rect) => ({
    top: rect.top - base.top,
    left: rect.left - base.left,
    width: rect.width,
    height: rect.height,
  }))
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

/**
 * The block a pointer over target picks to comment on whole: a table, a code
 * block, or a diagram as one, else the innermost block with source lines.
 * Null outside root or over a block with no text, such as a rule.
 */
export function pickableBlock(target: EventTarget | null, root: HTMLElement): HTMLElement | null {
  if (!(target instanceof Element)) return null
  const block =
    target.closest<HTMLElement>(
      'table[data-source-start], .markdown-code[data-source-start], .canvas-mermaid[data-source-start]',
    ) ?? target.closest<HTMLElement>('[data-source-start]')
  if (!block || !root.contains(block) || block === root) return null
  return blockTextRange(block) ? block : null
}

/**
 * The range a selection of the whole block holds: its text from the first
 * visible character to the last, so the narrowing in selectionToRange lands
 * on the block's first and last lines. A code block's copy button and a
 * diagram's style sheet stay out. Null for a block with no text.
 */
export function blockTextRange(block: HTMLElement): Range | null {
  const content = block.matches('.markdown-code') ? (block.querySelector('pre') ?? block) : block
  const walker = block.ownerDocument.createTreeWalker(content, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) =>
      node.parentElement?.closest('style, script, button') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT,
  })
  let start: { node: Text; offset: number } | null = null
  let end: { node: Text; offset: number } | null = null
  for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
    const first = node.data.search(/\S/)
    if (first < 0) continue
    start ??= { node, offset: first }
    end = { node, offset: node.data.trimEnd().length }
  }
  if (!start || !end) return null
  const range = block.ownerDocument.createRange()
  range.setStart(start.node, start.offset)
  range.setEnd(end.node, end.offset)
  return range
}
