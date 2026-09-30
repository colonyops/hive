import type { PaneLayout, WindowState } from './terminalClient'

export interface PaneRect {
  paneId: string
  x: number
  y: number
  width: number
  height: number
}

/**
 * A border between two sibling cells. `at` is the border's column (axis x)
 * or row (axis y), `from`/`to` its extent along the other axis, and
 * `beforePanes`/`afterPanes` the leaves on either side of it.
 *
 * `before` is the pane a resize is addressed to. tmux sizes the target's cell
 * inside its nearest ancestor split on the same axis, so the target has to be
 * a leaf whose nearest such ancestor is this divider's node: the cell before
 * the divider when it is a leaf, else one of that cell's direct leaf children.
 * Empty when the cell has none - no pane can move that divider, so it is not
 * draggable. `origin` and `extent` are the cell's start and size along the
 * axis, `limit` the most it may grow to before the cell after it disappears.
 */
export interface PaneDivider {
  axis: 'x' | 'y'
  at: number
  from: number
  to: number
  before: string
  beforePanes: string[]
  afterPanes: string[]
  origin: number
  extent: number
  limit: number
}

/** The pane the keyboard goes to in a window: tmux's active pane, or its only one. */
export function activePaneOf<T extends { paneId: string }>(tab: { activePane: string; panes: T[] }): T | undefined {
  return tab.panes.find((pane) => pane.paneId === tab.activePane) ?? tab.panes[0]
}

/** Every pane cell, left to right and top to bottom. */
export function layoutLeaves(layout: PaneLayout | null | undefined): PaneLayout[] {
  if (!layout) return []
  if (layout.paneId) return [layout]
  return (layout.cells ?? []).flatMap(layoutLeaves)
}

/** The pane ids of a window, in layout order; the active pane alone when tmux has reported no layout yet. */
export function windowPanes(state: Pick<WindowState, 'layout' | 'activePane'>): string[] {
  const leaves = layoutLeaves(state.layout).map((leaf) => leaf.paneId as string)
  if (leaves.length) return leaves
  return state.activePane ? [state.activePane] : []
}

/**
 * The grid each pane renders at: its layout cell, or the whole window for the
 * zoomed active pane. A pane hidden by zoom keeps its layout size because tmux
 * keeps its screen at that size. Without a layout, the active pane uses the
 * window grid.
 */
export function paneGrids(state: Pick<WindowState, 'layout' | 'activePane' | 'zoomed' | 'width' | 'height'>): Map<string, { cols: number; rows: number }> {
  const grids = new Map<string, { cols: number; rows: number }>()
  const leaves = layoutLeaves(state.layout)
  if (!leaves.length) {
    if (state.activePane) grids.set(state.activePane, { cols: state.width, rows: state.height })
    return grids
  }
  for (const leaf of leaves) {
    const zoomedIn = state.zoomed && leaf.paneId === state.activePane
    grids.set(leaf.paneId as string, zoomedIn
      ? { cols: state.width, rows: state.height }
      : { cols: leaf.width, rows: leaf.height })
  }
  return grids
}

/**
 * Where each pane on screen is drawn, in cells from the window's top left.
 * Under zoom only the active pane is on screen, over the whole window.
 */
export function visiblePaneRects(state: Pick<WindowState, 'layout' | 'activePane' | 'zoomed' | 'width' | 'height'>): PaneRect[] {
  const leaves = layoutLeaves(state.layout)
  if (!leaves.length) {
    return state.activePane ? [{ paneId: state.activePane, x: 0, y: 0, width: state.width, height: state.height }] : []
  }
  if (state.zoomed) {
    return leaves.some((leaf) => leaf.paneId === state.activePane)
      ? [{ paneId: state.activePane, x: 0, y: 0, width: state.width, height: state.height }]
      : []
  }
  return leaves.map((leaf) => ({ paneId: leaf.paneId as string, x: leaf.x, y: leaf.y, width: leaf.width, height: leaf.height }))
}

/** The dividers of an unzoomed layout; a zoomed window has none on screen. */
export function paneDividers(state: Pick<WindowState, 'layout' | 'zoomed'>): PaneDivider[] {
  if (!state.layout || state.zoomed) return []
  const dividers: PaneDivider[] = []
  collectDividers(state.layout, dividers)
  return dividers
}

function collectDividers(node: PaneLayout, out: PaneDivider[]): void {
  const cells = node.cells ?? []
  if (!node.split || cells.length < 2) return
  const horizontal = node.split === 'leftright'
  for (let i = 0; i < cells.length - 1; i++) {
    const before = cells[i]
    const after = cells[i + 1]
    const extent = horizontal ? before.width : before.height
    const next = horizontal ? after.width : after.height
    out.push({
      axis: horizontal ? 'x' : 'y',
      at: horizontal ? before.x + before.width : before.y + before.height,
      from: horizontal ? node.y : node.x,
      to: horizontal ? node.y + node.height : node.x + node.width,
      before: resizeTarget(before),
      beforePanes: paneIds(before),
      afterPanes: paneIds(after),
      origin: horizontal ? before.x : before.y,
      extent,
      limit: extent + next - 1,
    })
  }
  for (const cell of cells) collectDividers(cell, out)
}

// tmux never nests a split directly inside one on the same axis, so a split
// cell's direct leaf children are the only leaves that resolve to its parent.
function resizeTarget(cell: PaneLayout): string {
  if (cell.paneId) return cell.paneId
  return (cell.cells ?? []).find((child) => child.paneId)?.paneId ?? ''
}

function paneIds(node: PaneLayout): string[] {
  return layoutLeaves(node).map((leaf) => leaf.paneId as string)
}

/**
 * The size the cell before a divider ends up at when the divider is dragged to
 * `position` (a column or row): at least one cell, and never so far that the
 * cell after it vanishes.
 */
export function draggedExtent(divider: PaneDivider, position: number): number {
  const wanted = Math.floor(position) - divider.origin
  return Math.min(Math.max(wanted, 1), divider.limit)
}

export function dividerTouches(divider: PaneDivider, paneId: string): boolean {
  return divider.beforePanes.includes(paneId) || divider.afterPanes.includes(paneId)
}
