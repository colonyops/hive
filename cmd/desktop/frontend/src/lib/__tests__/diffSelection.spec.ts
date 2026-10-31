import { describe, expect, it } from 'vitest'
import { diffTextSelection } from '../diffSelection'

function mountDiff(): ShadowRoot {
  const host = document.createElement('div')
  document.body.append(host)
  const root = host.attachShadow({ mode: 'open' })
  root.innerHTML = `
    <code data-unified>
      <div data-line="4" data-line-type="change-addition"><span>const </span><span>first</span></div>
      <div data-line="5" data-line-type="change-deletion"><span>second</span></div>
      <div data-line="6" data-alt-line="6" data-line-type="context"><span>third</span></div>
    </code>`
  return root
}

// jsdom has no getComposedRanges, so a fake hands back what WebKit reports.
function selectionOf(range: Range | null): Selection {
  return {
    getComposedRanges: () =>
      range
        ? [
            {
              startContainer: range.startContainer,
              startOffset: range.startOffset,
              endContainer: range.endContainer,
              endOffset: range.endOffset,
              collapsed: range.collapsed,
            },
          ]
        : [],
    // WebKit says this about any selection inside a shadow root, which is why it goes unread.
    isCollapsed: true,
  } as unknown as Selection
}

function rangeOver(root: ShadowRoot, from: number, to: number): Range {
  const lines = root.querySelectorAll('[data-line]')
  const range = document.createRange()
  range.setStart(lines[from].firstChild!.firstChild!, 0)
  range.setEnd(lines[to].lastChild!.firstChild!, 3)
  return range
}

describe('diffTextSelection', () => {
  it('gives the exact text and its line for a selection on one line', () => {
    const root = mountDiff()
    const got = diffTextSelection(selectionOf(rangeOver(root, 0, 0)), [{ path: 'a.go', root }])
    expect(got).toMatchObject({
      path: 'a.go',
      text: 'const fir',
      range: { start: 4, side: 'additions', end: 4, endSide: 'additions' },
    })
  })

  it('leaves the text to the diff rows across lines, and reads each end its side', () => {
    const root = mountDiff()
    const got = diffTextSelection(selectionOf(rangeOver(root, 0, 1)), [{ path: 'a.go', root }])
    expect(got?.text).toBeUndefined()
    expect(got?.range).toEqual({ start: 4, side: 'additions', end: 5, endSide: 'deletions' })
  })

  it('is null for nothing selected, or a selection outside the mounted files', () => {
    const root = mountDiff()
    expect(diffTextSelection(selectionOf(null), [{ path: 'a.go', root }])).toBeNull()
    const collapsed = rangeOver(root, 0, 0)
    collapsed.collapse(true)
    expect(diffTextSelection(selectionOf(collapsed), [{ path: 'a.go', root }])).toBeNull()
    expect(diffTextSelection(selectionOf(rangeOver(root, 0, 0)), [])).toBeNull()
  })
})
