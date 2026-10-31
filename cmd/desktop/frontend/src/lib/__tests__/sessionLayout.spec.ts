import { describe, expect, it } from 'vitest'
import {
  MAX_SPLIT_RATIO,
  MIN_SPLIT_RATIO,
  addViewQuietly,
  closePane,
  closeTab,
  focusedTab,
  focusedWindow,
  inTabOrder,
  isSessionLayout,
  moveTab,
  moveTabBeside,
  newSessionLayout,
  offscreenWindows,
  openView,
  orderedTabs,
  paneBoxes,
  panesOf,
  placeView,
  placeWindow,
  reconcileWindows,
  resizeSplit,
  revealWindow,
  selectTab,
  splitBoxes,
  splitPane,
  terminalPane,
  turnSplit,
  windowOrder,
  windowsOnScreen,
  type SessionLayout,
} from '../sessionLayout'

function attached(...windows: string[]): SessionLayout {
  const list = windows.length ? windows : ['@1']
  return reconcileWindows(newSessionLayout(), list, { active: list[0] })
}

const ids = (layout: SessionLayout, pane = 0) => panesOf(layout)[pane].tabs.map((tab) => tab.id)
const paneId = (layout: SessionLayout, pane = 0) => panesOf(layout)[pane].id

function splitWith(layout: SessionLayout, windowId: string): SessionLayout {
  const split = splitPane(layout, paneId(layout), 'row')
  return placeWindow(split, windowId, paneId(split, 1))
}

describe('a tab per window', () => {
  it('gives every window a tab, in order', () => {
    const layout = attached('@1', '@2', '@3')
    expect(ids(layout)).toEqual(['window:@1', 'window:@2', 'window:@3'])
    expect(windowOrder(layout)).toEqual(['@1', '@2', '@3'])
  })

  it('shows the active window first', () => {
    expect(focusedWindow(reconcileWindows(newSessionLayout(), ['@1', '@2'], { active: '@2' }))).toBe('@2')
  })

  it('adds a tab for a new window in the focused pane, or the pane that asked', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    expect(ids(reconcileWindows(split, ['@1', '@2', '@3']), 1)).toEqual(['window:@2', 'window:@3'])
    const asked = reconcileWindows(split, ['@1', '@2', '@3'], { target: paneId(split, 0) })
    expect(ids(asked)).toEqual(['window:@1', 'window:@3'])
  })

  it('takes the tab away with its window, and moves to a neighbour', () => {
    const start = attached('@1', '@2')
    const layout = reconcileWindows(selectTab(start, paneId(start), 'window:@2'), ['@1'])
    expect(ids(layout)).toEqual(['window:@1'])
    expect(panesOf(layout)[0].activeTab).toBe('window:@1')
  })

  it('closes a pane whose only window went', () => {
    expect(panesOf(reconcileWindows(splitWith(attached('@1', '@2'), '@2'), ['@1']))).toHaveLength(1)
  })

  it('leaves views alone when a session has no windows left', () => {
    expect(ids(reconcileWindows(openView(attached('@1'), 'docs'), []))).toEqual(['docs'])
  })

  it('returns the same layout when nothing changed, so a poll costs no write', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(reconcileWindows(layout, ['@1', '@2'])).toBe(layout)
  })

  it('leaves a pane that is still asking alone', () => {
    const layout = splitPane(attached('@1'), 'p0', 'row')
    expect(reconcileWindows(layout, ['@1'])).toBe(layout)
  })

  it('reveals a window where its tab lives', () => {
    const start = splitWith(attached('@1', '@2'), '@2')
    const revealed = revealWindow(selectTab(start, paneId(start), 'window:@1'), '@2')
    expect(revealed.focusedPane).toBe(paneId(start, 1))
    expect(focusedWindow(revealed)).toBe('@2')
  })

  it('makes no tab for a window that has not landed, so tree order holds', () => {
    const layout = newSessionLayout()
    expect(revealWindow(layout, '@2')).toBe(layout)
  })

  it('brings the terminal forward from behind a view', () => {
    expect(panesOf(revealWindow(openView(attached('@1'), 'docs'), '@1'))[0].activeTab).toBe('window:@1')
  })
})

describe('the tree mirrors the tabs', () => {
  it('lists every tab once, in tab order, across panes', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    const layout = placeView(openView(split, 'docs', paneId(split, 0)), 'tasks', paneId(split, 1))
    expect(orderedTabs(layout).map(({ tab }) => tab.id)).toEqual(['window:@1', 'docs', 'window:@2', 'tasks'])
  })

  it('follows a view as it opens and closes', () => {
    const opened = openView(attached(), 'diff')
    expect(orderedTabs(opened)).toHaveLength(2)
    expect(orderedTabs(closeTab(opened, 'p0', 'diff'))).toHaveLength(1)
  })

  it('follows a drag in the tabs', () => {
    expect(windowOrder(moveTab(attached('@1', '@2', '@3'), 'window:@3', 'p0', 0))).toEqual(['@3', '@1', '@2'])
  })

  it('moves any tab when a tree row drops beside another, views included', () => {
    const layout = openView(attached('@1', '@2'), 'docs')
    expect(ids(moveTabBeside(layout, 'window:@2', 'window:@1', 'before'))).toEqual(['window:@2', 'window:@1', 'docs'])
    expect(ids(moveTabBeside(layout, 'docs', 'window:@1', 'before'))).toEqual(['docs', 'window:@1', 'window:@2'])
    expect(ids(moveTabBeside(layout, 'window:@1', 'docs', 'after'))).toEqual(['window:@2', 'docs', 'window:@1'])
  })
})

describe('tabs', () => {
  it('focuses a view that is already open instead of opening it twice', () => {
    const again = openView(selectTab(openView(attached(), 'diff'), 'p0', 'window:@1'), 'diff')
    expect(ids(again)).toEqual(['window:@1', 'diff'])
    expect(panesOf(again)[0].activeTab).toBe('diff')
  })

  it('focuses a view open in another pane rather than adding it to this one', () => {
    const split = splitPane(attached(), 'p0', 'row')
    const again = openView(placeView(split, 'diff', paneId(split, 1)), 'diff', 'p0')
    expect(again.focusedPane).toBe(paneId(split, 1))
    expect(ids(again)).toEqual(['window:@1'])
  })

  it('moves to the neighbouring tab when the active one closes', () => {
    expect(panesOf(closeTab(openView(openView(attached(), 'docs'), 'diff'), 'p0', 'diff'))[0].activeTab).toBe('docs')
  })
})

describe('nested splits', () => {
  it('splits a pane and asks what goes in the new one', () => {
    const layout = splitPane(attached(), 'p0', 'row')
    expect(panesOf(layout)).toHaveLength(2)
    expect(panesOf(layout)[1].picking).toBe(true)
    expect(layout.focusedPane).toBe(paneId(layout, 1))
  })

  it('splits within a split, as deep as asked, with its own direction', () => {
    const one = splitPane(attached(), 'p0', 'row')
    const two = splitPane(one, paneId(one, 1), 'column')
    const three = splitPane(two, 'p0', 'column')
    expect(panesOf(three)).toHaveLength(4)
    expect(splitBoxes(three).map(({ split }) => split.axis)).toEqual(['row', 'column', 'column'])
    expect(new Set(panesOf(three).map((pane) => pane.id)).size).toBe(4)
  })

  it('places panes in boxes that fill the session without overlap', () => {
    const one = splitPane(attached(), 'p0', 'row')
    const two = splitPane(one, paneId(one, 1), 'column')
    expect([...paneBoxes(two).values()]).toEqual([
      { x: 0, y: 0, width: 0.5, height: 1 },
      { x: 0.5, y: 0, width: 0.5, height: 0.5 },
      { x: 0.5, y: 0.5, width: 0.5, height: 0.5 },
    ])
  })

  it('places a nested boundary inside the box it divides', () => {
    const one = splitPane(attached(), 'p0', 'row')
    const two = splitPane(one, paneId(one, 1), 'column')
    expect(splitBoxes(two)[1].box).toEqual({ x: 0.5, y: 0, width: 0.5, height: 1 })
  })

  it('collapses only the split that lost a pane', () => {
    const one = splitPane(attached('@1', '@2', '@3'), 'p0', 'row')
    const two = splitPane(placeWindow(one, '@2', paneId(one, 1)), paneId(one, 1), 'column')
    const three = placeWindow(two, '@3', paneId(two, 2))
    const closed = closeTab(placeView(three, 'docs', paneId(three, 2)), paneId(three, 2), 'docs')
    expect(panesOf(closed)).toHaveLength(3)
    const gone = reconcileWindows(three, ['@1', '@2'])
    expect(panesOf(gone)).toHaveLength(2)
    expect(splitBoxes(gone)).toHaveLength(1)
    expect(splitBoxes(gone)[0].split.axis).toBe('row')
  })

  it('closes a pane by merging its tabs into its neighbour, so a window keeps its tab', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    const closed = closePane(layout, paneId(layout, 1))
    expect(panesOf(closed)).toHaveLength(1)
    expect(ids(closed)).toEqual(['window:@1', 'window:@2'])
    expect(closed.focusedPane).toBe(paneId(layout, 0))
  })

  it('cancels a picker without opening another', () => {
    const lone = closeTab(splitPane(openView(newSessionLayout(), 'docs'), 'p0', 'row'), 'p0', 'docs')
    expect(panesOf(lone)).toHaveLength(1)
    const cancelled = closePane(lone, paneId(lone))
    expect(panesOf(cancelled)).toHaveLength(1)
    expect(panesOf(cancelled)[0].picking).toBe(false)
  })

  it('moves focus off a pane that closes', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(reconcileWindows(layout, ['@1']).focusedPane).toBe(paneId(layout, 0))
  })

  it('moves a view out of a pane when the picker names it', () => {
    const split = splitPane(openView(attached(), 'diff'), 'p0', 'row')
    const layout = placeView(split, 'diff', paneId(split, 1))
    expect(ids(layout)).toEqual(['window:@1'])
    expect(panesOf(layout)[0].activeTab).toBe('window:@1')
    expect(ids(layout, 1)).toEqual(['diff'])
    expect(panesOf(layout)[1].picking).toBe(false)
  })

  it('moves a window tab when the picker names it, so it stays one tab', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(ids(layout)).toEqual(['window:@1'])
    expect(ids(placeWindow(layout, '@2', paneId(layout, 1)), 1)).toEqual(['window:@2'])
  })

  it('offers only windows behind another tab', () => {
    expect(offscreenWindows(splitPane(attached('@1', '@2', '@3'), 'p0', 'row'), ['@1', '@2', '@3'])).toEqual([
      '@2',
      '@3',
    ])
  })

  it('reports what each pane shows, and nothing behind a view', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(windowsOnScreen(layout).get('@2')).toBe(paneId(layout, 1))
    expect(windowsOnScreen(openView(layout, 'docs', 'p0')).has('@1')).toBe(false)
  })

  it('names the pane where a terminal state shows', () => {
    expect(terminalPane(newSessionLayout())).toBe('p0')
    expect(terminalPane(openView(attached(), 'docs'))).toBe('')
  })
})

describe('moveTab', () => {
  const three = () => openView(openView(openView(attached(), 'docs'), 'diff'), 'tasks')

  it('reorders within a row', () => {
    expect(ids(moveTab(three(), 'tasks', 'p0', 1))).toEqual(['window:@1', 'tasks', 'docs', 'diff'])
    expect(ids(moveTab(three(), 'docs', 'p0', 4))).toEqual(['window:@1', 'diff', 'tasks', 'docs'])
  })

  it('reads the gap before the move, so dropping beside itself changes nothing', () => {
    expect(ids(moveTab(three(), 'docs', 'p0', 2))).toEqual(ids(three()))
    expect(ids(moveTab(three(), 'docs', 'p0', 1))).toEqual(ids(three()))
  })

  it('moves a tab between panes and focuses it there', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    const layout = moveTab(openView(split, 'docs', 'p0'), 'docs', paneId(split, 1), 0)
    expect(ids(layout)).toEqual(['window:@1'])
    expect(ids(layout, 1)).toEqual(['docs', 'window:@2'])
    expect(layout.focusedPane).toBe(paneId(split, 1))
    expect(focusedTab(layout)).toBe('docs')
  })

  it('closes the pane its last tab left, on either side', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    const layout = moveTab(split, 'window:@1', paneId(split, 1), 0)
    expect(panesOf(layout)).toHaveLength(1)
    expect(ids(layout)).toEqual(['window:@1', 'window:@2'])
  })

  it('keeps a picker on screen when a window nobody asked for arrives', () => {
    const asking = splitPane(attached('@1', '@2'), 'p0', 'row')
    const layout = reconcileWindows(asking, ['@1', '@2', '@3'], { active: '@1' })
    const pane = panesOf(layout).find((candidate) => candidate.id === asking.focusedPane)!
    expect(pane.picking).toBe(true)
    expect(pane.tabs.map((tab) => tab.id)).toContain('window:@3')
  })

  it('lets a window the pane asked for answer its picker', () => {
    const asking = splitPane(attached('@1', '@2'), 'p0', 'row')
    const layout = reconcileWindows(asking, ['@1', '@2', '@3'], { active: '@1', target: asking.focusedPane })
    expect(panesOf(layout).find((candidate) => candidate.id === asking.focusedPane)!.picking).toBe(false)
  })

  it('orders windows as the tabs show them, not as tmux does', () => {
    const split = splitPane(attached('@1', '@2'), 'p0', 'row')
    const layout = closePane(placeWindow(split, '@2', paneId(split, 1)), 'p0')
    expect(windowOrder(layout)).toEqual(['@2', '@1'])
    const tmuxOrder = [{ windowId: '@1' }, { windowId: '@2' }, { windowId: '@9' }]
    expect(inTabOrder(tmuxOrder, layout).map((window) => window.windowId)).toEqual(['@2', '@1', '@9'])
  })

  it('fills a pane that was asking', () => {
    const split = splitPane(openView(attached(), 'diff'), 'p0', 'row')
    const layout = moveTab(split, 'diff', paneId(split, 1), 0)
    expect(panesOf(layout)[1].picking).toBe(false)
    expect(ids(layout, 1)).toEqual(['diff'])
  })

  it('ignores a tab or a pane that is not there', () => {
    const layout = three()
    expect(moveTab(layout, 'nope', 'p0', 1)).toBe(layout)
    expect(moveTab(layout, 'docs', 'nope', 0)).toBe(layout)
  })
})

describe('addViewQuietly', () => {
  it('adds the tab without taking the screen or the keyboard', () => {
    const layout = addViewQuietly(attached('@1'), 'docs')
    expect(ids(layout)).toEqual(['window:@1', 'docs'])
    expect(focusedWindow(layout)).toBe('@1')
  })

  it('leaves a tab that is already open alone', () => {
    const layout = openView(attached('@1'), 'docs')
    expect(addViewQuietly(layout, 'docs')).toBe(layout)
  })

  it('joins the last pane, behind its picker', () => {
    const layout = addViewQuietly(splitPane(attached('@1'), 'p0', 'row'), 'docs')
    expect(ids(layout, 1)).toEqual(['docs'])
    expect(panesOf(layout)[1].picking).toBe(true)
  })
})

describe('resizing and turning', () => {
  const split = () => splitPane(attached(), 'p0', 'row')
  const splitId = (layout: SessionLayout) => splitBoxes(layout)[0].split.id

  it('clamps so neither side can vanish', () => {
    const layout = split()
    expect(splitBoxes(resizeSplit(layout, splitId(layout), 0.01))[0].split.ratio).toBe(MIN_SPLIT_RATIO)
    expect(splitBoxes(resizeSplit(layout, splitId(layout), 0.99))[0].split.ratio).toBe(MAX_SPLIT_RATIO)
    expect(splitBoxes(resizeSplit(layout, splitId(layout), 0.3))[0].split.ratio).toBe(0.3)
  })

  it('returns the same layout at the same size', () => {
    const layout = split()
    expect(resizeSplit(layout, splitId(layout), 0.5)).toBe(layout)
  })

  it('resizes one split and leaves the others', () => {
    const one = split()
    const two = splitPane(one, paneId(one, 1), 'column')
    const [outer, inner] = splitBoxes(two)
    const sized = resizeSplit(two, inner.split.id, 0.3)
    expect(splitBoxes(sized)[0].split.ratio).toBe(outer.split.ratio)
    expect(splitBoxes(sized)[1].split.ratio).toBe(0.3)
  })

  it('turns one split with its contents in place', () => {
    const open = placeView(split(), 'diff', 'p1')
    const turned = turnSplit(open, splitId(open))
    expect(splitBoxes(turned)[0].split.axis).toBe('column')
    expect(ids(turned, 1)).toEqual(['diff'])
  })
})

describe('isSessionLayout', () => {
  it('accepts a real layout and refuses what storage might hold instead', () => {
    expect(isSessionLayout(splitPane(attached(), 'p0', 'row'))).toBe(true)
    expect(isSessionLayout(null)).toBe(false)
    expect(isSessionLayout({ panes: [] })).toBe(false)
    expect(isSessionLayout({ root: { kind: 'split', ratio: 0.5, axis: 'row' }, focusedPane: 'p0', nextId: 1 })).toBe(
      false,
    )
  })
})
