import { describe, expect, it } from 'vitest'
import type { PaneLayout } from '../terminalClient'
import {
  dividerTouches,
  draggedExtent,
  layoutLeaves,
  paneDividers,
  paneGrids,
  visiblePaneRects,
  windowPanes,
} from '../terminalLayout'

// Recorded from tmux 3.7b: split-window -h, then -v on the right pane.
//   95e4,120x40,0,0{60x40,0,0,0,59x40,61,0[59x20,61,0,1,59x19,61,21,2]}
const nested: PaneLayout = {
  split: 'leftright', x: 0, y: 0, width: 120, height: 40,
  cells: [
    { paneId: '%0', x: 0, y: 0, width: 60, height: 40 },
    {
      split: 'topbottom', x: 61, y: 0, width: 59, height: 40,
      cells: [
        { paneId: '%1', x: 61, y: 0, width: 59, height: 20 },
        { paneId: '%2', x: 61, y: 21, width: 59, height: 19 },
      ],
    },
  ],
}

const single: PaneLayout = { paneId: '%7', x: 0, y: 0, width: 120, height: 40 }

// Recorded from tmux 3.7b: split-window -h, then -v on the left pane, then -h
// on its top pane. The root and the innermost split share an axis and a first
// leaf, which is what tells a divider's own panes apart from an ancestor's.
//   524c,120x40,0,0{60x40,0,0[60x20,0,0{30x20,0,0,0,29x20,31,0,3},60x19,0,21,2],59x40,61,0,1}
const threeLevel: PaneLayout = {
  split: 'leftright', x: 0, y: 0, width: 120, height: 40,
  cells: [
    {
      split: 'topbottom', x: 0, y: 0, width: 60, height: 40,
      cells: [
        {
          split: 'leftright', x: 0, y: 0, width: 60, height: 20,
          cells: [
            { paneId: '%0', x: 0, y: 0, width: 30, height: 20 },
            { paneId: '%3', x: 31, y: 0, width: 29, height: 20 },
          ],
        },
        { paneId: '%2', x: 0, y: 21, width: 60, height: 19 },
      ],
    },
    { paneId: '%1', x: 61, y: 0, width: 59, height: 40 },
  ],
}

// Recorded from tmux 3.7b: threeLevel, then -h on the bottom-left pane. Both
// cells stacked on the left are splits, so no pane's nearest leftright ancestor
// is the root: resize-pane cannot move the root border through any of them.
//   ef26,120x40,0,0{60x40,0,0[60x20,0,0{30x20,0,0,0,29x20,31,0,3},60x19,0,21{30x19,0,21,2,29x19,31,21,4}],59x40,61,0,1}
const unreachable: PaneLayout = {
  split: 'leftright', x: 0, y: 0, width: 120, height: 40,
  cells: [
    {
      split: 'topbottom', x: 0, y: 0, width: 60, height: 40,
      cells: [
        {
          split: 'leftright', x: 0, y: 0, width: 60, height: 20,
          cells: [
            { paneId: '%0', x: 0, y: 0, width: 30, height: 20 },
            { paneId: '%3', x: 31, y: 0, width: 29, height: 20 },
          ],
        },
        {
          split: 'leftright', x: 0, y: 21, width: 60, height: 19,
          cells: [
            { paneId: '%2', x: 0, y: 21, width: 30, height: 19 },
            { paneId: '%4', x: 31, y: 21, width: 29, height: 19 },
          ],
        },
      ],
    },
    { paneId: '%1', x: 61, y: 0, width: 59, height: 40 },
  ],
}

describe('terminalLayout', () => {
  it('lists leaves left to right, then top to bottom', () => {
    expect(layoutLeaves(nested).map((leaf) => leaf.paneId)).toEqual(['%0', '%1', '%2'])
    expect(layoutLeaves(null)).toEqual([])
  })

  it('names the active pane alone when tmux has reported no layout yet', () => {
    expect(windowPanes({ layout: null, activePane: '%3' })).toEqual(['%3'])
    expect(windowPanes({ layout: null, activePane: '' })).toEqual([])
    expect(windowPanes({ layout: nested, activePane: '%1' })).toEqual(['%0', '%1', '%2'])
  })

  it('gives each pane its cell, and a zoomed pane the whole window', () => {
    const unzoomed = paneGrids({ layout: nested, activePane: '%1', zoomed: false, width: 120, height: 40 })
    expect(unzoomed.get('%0')).toEqual({ cols: 60, rows: 40 })
    expect(unzoomed.get('%2')).toEqual({ cols: 59, rows: 19 })

    const zoomed = paneGrids({ layout: nested, activePane: '%1', zoomed: true, width: 120, height: 40 })
    expect(zoomed.get('%1')).toEqual({ cols: 120, rows: 40 })
    // Hidden panes retain the grid tmux uses for their screens.
    expect(zoomed.get('%2')).toEqual({ cols: 59, rows: 19 })

    const unreported = paneGrids({ layout: null, activePane: '%3', zoomed: false, width: 80, height: 24 })
    expect(unreported.get('%3')).toEqual({ cols: 80, rows: 24 })
  })

  it('places every pane unzoomed and only the active one zoomed', () => {
    expect(visiblePaneRects({ layout: nested, activePane: '%1', zoomed: false, width: 120, height: 40 })).toEqual([
      { paneId: '%0', x: 0, y: 0, width: 60, height: 40 },
      { paneId: '%1', x: 61, y: 0, width: 59, height: 20 },
      { paneId: '%2', x: 61, y: 21, width: 59, height: 19 },
    ])
    expect(visiblePaneRects({ layout: nested, activePane: '%2', zoomed: true, width: 120, height: 40 })).toEqual([
      { paneId: '%2', x: 0, y: 0, width: 120, height: 40 },
    ])
    expect(visiblePaneRects({ layout: single, activePane: '%7', zoomed: false, width: 120, height: 40 })).toEqual([
      { paneId: '%7', x: 0, y: 0, width: 120, height: 40 },
    ])
  })

  it('finds the border between every pair of siblings, addressed to the cell before it', () => {
    const dividers = paneDividers({ layout: nested, zoomed: false })
    expect(dividers).toEqual([
      { axis: 'x', at: 60, from: 0, to: 40, before: '%0', beforePanes: ['%0'], afterPanes: ['%1', '%2'], origin: 0, extent: 60, limit: 118 },
      { axis: 'y', at: 20, from: 61, to: 120, before: '%1', beforePanes: ['%1'], afterPanes: ['%2'], origin: 0, extent: 20, limit: 38 },
    ])
    expect(paneDividers({ layout: nested, zoomed: true })).toEqual([])
    expect(paneDividers({ layout: single, zoomed: false })).toEqual([])
  })

  // tmux sizes the target's cell inside its nearest same-axis ancestor, so a
  // divider whose before cell is itself split has to be addressed through one
  // of that cell's direct leaves - never its first leaf, which may sit under
  // a nested split on the same axis and would move that split's border.
  it('addresses a divider to a leaf whose nearest same-axis ancestor is the divider itself', () => {
    const [root, middle, inner] = paneDividers({ layout: threeLevel, zoomed: false })
    expect(root).toMatchObject({ axis: 'x', at: 60, before: '%2', beforePanes: ['%0', '%3', '%2'], afterPanes: ['%1'], extent: 60, limit: 118 })
    expect(middle).toMatchObject({ axis: 'y', at: 20, from: 0, to: 60, before: '%0', beforePanes: ['%0', '%3'], afterPanes: ['%2'], extent: 20, limit: 38 })
    expect(inner).toMatchObject({ axis: 'x', at: 30, from: 0, to: 20, before: '%0', beforePanes: ['%0'], afterPanes: ['%3'], extent: 30, limit: 58 })
  })

  it('leaves a divider unaddressed when no pane can reach it', () => {
    const [root, middle, top, bottom] = paneDividers({ layout: unreachable, zoomed: false })
    expect(root).toMatchObject({ axis: 'x', at: 60, before: '', beforePanes: ['%0', '%3', '%2', '%4'], afterPanes: ['%1'] })
    expect(middle).toMatchObject({ axis: 'y', at: 20, before: '%0' })
    expect(top).toMatchObject({ axis: 'x', at: 30, from: 0, to: 20, before: '%0' })
    expect(bottom).toMatchObject({ axis: 'x', at: 30, from: 21, to: 40, before: '%2' })
  })

  it('clamps a dragged divider to one cell either side', () => {
    const [outer] = paneDividers({ layout: nested, zoomed: false })
    expect(draggedExtent(outer, 30.4)).toBe(30)
    // The line is drawn down the middle of the border cell, so a grab there
    // reads as the cell's own column, not the next one.
    expect(draggedExtent(outer, 60.6)).toBe(60)
    expect(draggedExtent(outer, 0)).toBe(1)
    expect(draggedExtent(outer, 500)).toBe(118)
  })

  it('knows which borders the active pane touches', () => {
    const [outer, inner] = paneDividers({ layout: nested, zoomed: false })
    expect(dividerTouches(outer, '%0')).toBe(true)
    expect(dividerTouches(outer, '%2')).toBe(true)
    expect(dividerTouches(inner, '%0')).toBe(false)
    expect(dividerTouches(inner, '%2')).toBe(true)
    expect(dividerTouches(inner, '')).toBe(false)
  })

  it('tells an inner divider apart from an ancestor on the same axis', () => {
    const [root, middle, inner] = paneDividers({ layout: threeLevel, zoomed: false })
    expect(['%0', '%3', '%2', '%1'].map((pane) => dividerTouches(root, pane))).toEqual([true, true, true, true])
    expect(['%0', '%3', '%2', '%1'].map((pane) => dividerTouches(middle, pane))).toEqual([true, true, true, false])
    expect(['%0', '%3', '%2', '%1'].map((pane) => dividerTouches(inner, pane))).toEqual([true, true, false, false])
  })
})
