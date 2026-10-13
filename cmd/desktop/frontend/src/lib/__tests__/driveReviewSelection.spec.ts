import { afterEach, describe, expect, it } from 'vitest'
import { markReviewedBlocks, selectionToRange } from '../driveReviewSelection'

function mountRoot(html: string): HTMLElement {
  const root = document.createElement('div')
  root.innerHTML = html
  document.body.append(root)
  return root
}

function select(start: Node, startOffset: number, end: Node, endOffset: number): Selection {
  const range = document.createRange()
  range.setStart(start, startOffset)
  range.setEnd(end, endOffset)
  const selection = window.getSelection()!
  selection.removeAllRanges()
  selection.addRange(range)
  return selection
}

const text = (root: HTMLElement, selector: string) => root.querySelector(selector)!.firstChild!

const body = `
  <h1 data-source-start="4" data-source-end="4">Plan</h1>
  <p data-source-start="6" data-source-end="7">Step one.
Step two.</p>
  <ul data-source-start="9" data-source-end="11">
    <li data-source-start="9" data-source-end="9">a</li>
    <li data-source-start="10" data-source-end="11"><p>b</p></li>
  </ul>
  <div class="canvas-mermaid" data-source-start="13" data-source-end="15"><svg><text>Graph</text></svg></div>
`

afterEach(() => {
  window.getSelection()?.removeAllRanges()
  document.body.innerHTML = ''
})

describe('selectionToRange', () => {
  it('maps a selection inside one block to its lines', () => {
    const root = mountRoot(body)
    const node = text(root, 'p')
    expect(selectionToRange(select(node, 0, node, 9), root)).toEqual({ startLine: 6, endLine: 7, context: 'Step one.' })
  })

  it('spans the blocks at both ends of a selection', () => {
    const root = mountRoot(body)
    const range = selectionToRange(select(text(root, 'h1'), 0, text(root, 'p'), 4), root)
    expect(range).toMatchObject({ startLine: 4, endLine: 7 })
  })

  it('uses the innermost block, a nested list item', () => {
    const root = mountRoot(body)
    const node = text(root, 'li:nth-child(2) p')
    expect(selectionToRange(select(node, 0, node, 1), root)).toMatchObject({ startLine: 10, endLine: 11 })
  })

  it('ends in a mermaid part', () => {
    const root = mountRoot(body)
    const range = selectionToRange(select(text(root, 'p'), 0, root.querySelector('text')!.firstChild!, 5), root)
    expect(range).toMatchObject({ startLine: 6, endLine: 15 })
  })

  it('is null for a collapsed selection', () => {
    const root = mountRoot(body)
    const node = text(root, 'p')
    expect(selectionToRange(select(node, 2, node, 2), root)).toBeNull()
  })

  it('is null for a selection outside the root', () => {
    const root = mountRoot(body)
    const outside = mountRoot('<p data-source-start="1" data-source-end="1">elsewhere</p>')
    const node = text(outside, 'p')
    expect(selectionToRange(select(node, 0, node, 4), root)).toBeNull()
  })
})

describe('markReviewedBlocks', () => {
  it('marks the innermost overlapping blocks and the active comment', () => {
    const root = mountRoot(body)
    markReviewedBlocks(
      root,
      [
        { id: 'a', startLine: 4, endLine: 4 },
        { id: 'b', startLine: 9, endLine: 10 },
      ],
      'b',
    )
    expect(root.querySelector('h1')!.dataset.reviewIds).toBe('a')
    expect(root.querySelector('h1')!.classList.contains('is-active-review')).toBe(false)
    const items = root.querySelectorAll('li')
    expect(items[0].classList.contains('has-review-comment')).toBe(true)
    expect(items[0].classList.contains('is-active-review')).toBe(true)
    expect(items[1].dataset.reviewIds).toBe('b')
    expect(root.querySelector('ul')!.classList.contains('has-review-comment')).toBe(false)

    markReviewedBlocks(root, [])
    expect(root.querySelectorAll('.has-review-comment, .is-active-review')).toHaveLength(0)
    expect(root.querySelector('[data-review-ids]')).toBeNull()
  })
})
