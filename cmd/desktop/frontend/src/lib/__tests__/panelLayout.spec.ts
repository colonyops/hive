import { describe, expect, it } from 'vitest'
import {
  MAX_SPLIT_RATIO,
  MIN_SPLIT_RATIO,
  addTabBeside,
  addTabQuietly,
  closePanel,
  closeTab,
  focusPanel,
  focusedTab,
  isPanelLayout,
  moveTab,
  moveTabBeside,
  newPanelLayout,
  openTab,
  orderedTabs,
  panelBoxes,
  panelsOf,
  placeTab,
  resizeSplit,
  selectTab,
  shownTabs,
  splitBoxes,
  splitPanel,
  syncTabs,
  tabById,
  turnSplit,
  withTabParams,
  type PanelLayout,
  type PanelTab,
} from '../panelLayout'

const win = (id: string): PanelTab => ({ id: `window:${id}`, kind: 'window', params: { windowId: id } })
const view = (kind: string): PanelTab => ({ id: kind, kind })

function attached(...windows: string[]): PanelLayout {
  const list = windows.length ? windows : ['@1']
  return syncTabs(newPanelLayout(), 'window', list, win, { active: `window:${list[0]}` })
}

const ids = (layout: PanelLayout, panel = 0) => panelsOf(layout)[panel].tabs.map((tab) => tab.id)
const panelId = (layout: PanelLayout, panel = 0) => panelsOf(layout)[panel].id

function splitWith(layout: PanelLayout, windowId: string): PanelLayout {
  const split = splitPanel(layout, panelId(layout), 'row')
  return placeTab(split, win(windowId), panelId(split, 1))
}

describe('syncTabs', () => {
  it('gives every id a tab, in order', () => {
    expect(ids(attached('@1', '@2', '@3'))).toEqual(['window:@1', 'window:@2', 'window:@3'])
  })

  it('shows the active tab first', () => {
    const layout = syncTabs(newPanelLayout(), 'window', ['@1', '@2'], win, { active: 'window:@2' })
    expect(focusedTab(layout)).toBe('window:@2')
  })

  it('adds a tab for a new id in the focused panel, or the panel that asked', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    expect(ids(syncTabs(split, 'window', ['@1', '@2', '@3'], win), 1)).toEqual(['window:@2', 'window:@3'])
    const asked = syncTabs(split, 'window', ['@1', '@2', '@3'], win, { target: panelId(split, 0) })
    expect(ids(asked)).toEqual(['window:@1', 'window:@3'])
  })

  it('takes the tab away with its id, and moves to a neighbour', () => {
    const start = attached('@1', '@2')
    const layout = syncTabs(selectTab(start, panelId(start), 'window:@2'), 'window', ['@1'], win)
    expect(ids(layout)).toEqual(['window:@1'])
    expect(panelsOf(layout)[0].activeTab).toBe('window:@1')
  })

  it('closes a panel whose only tab went', () => {
    expect(panelsOf(syncTabs(splitWith(attached('@1', '@2'), '@2'), 'window', ['@1'], win))).toHaveLength(1)
  })

  it('leaves tabs of another kind alone when the set is empty', () => {
    expect(ids(syncTabs(openTab(attached('@1'), view('docs')), 'window', [], win))).toEqual(['docs'])
  })

  it('returns the same layout when nothing changed, so a poll costs no write', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(syncTabs(layout, 'window', ['@1', '@2'], win)).toBe(layout)
  })

  it('leaves a panel that is still asking alone', () => {
    const layout = splitPanel(attached('@1'), 'p0', 'row')
    expect(syncTabs(layout, 'window', ['@1'], win)).toBe(layout)
  })

  it('keeps a picker on screen when a tab nobody asked for arrives', () => {
    const asking = splitPanel(attached('@1', '@2'), 'p0', 'row')
    const layout = syncTabs(asking, 'window', ['@1', '@2', '@3'], win, { active: 'window:@1' })
    const panel = panelsOf(layout).find((candidate) => candidate.id === asking.focusedPanel)!
    expect(panel.picking).toBe(true)
    expect(panel.tabs.map((tab) => tab.id)).toContain('window:@3')
  })

  it('lets a tab the panel asked for answer its picker', () => {
    const asking = splitPanel(attached('@1', '@2'), 'p0', 'row')
    const layout = syncTabs(asking, 'window', ['@1', '@2', '@3'], win, {
      active: 'window:@1',
      target: asking.focusedPanel,
    })
    expect(panelsOf(layout).find((candidate) => candidate.id === asking.focusedPanel)!.picking).toBe(false)
  })

  it('moves focus off a panel that closes', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(syncTabs(layout, 'window', ['@1'], win).focusedPanel).toBe(panelId(layout, 0))
  })
})

describe('tabs', () => {
  it('focuses a tab that is already open instead of opening it twice', () => {
    const again = openTab(selectTab(openTab(attached(), view('diff')), 'p0', 'window:@1'), view('diff'))
    expect(ids(again)).toEqual(['window:@1', 'diff'])
    expect(panelsOf(again)[0].activeTab).toBe('diff')
  })

  it('focuses a tab open in another panel rather than adding it to this one', () => {
    const split = splitPanel(attached(), 'p0', 'row')
    const again = openTab(placeTab(split, view('diff'), panelId(split, 1)), view('diff'), 'p0')
    expect(again.focusedPanel).toBe(panelId(split, 1))
    expect(ids(again)).toEqual(['window:@1'])
  })

  it('opens in the first panel when the target is gone', () => {
    expect(ids(openTab(attached(), view('diff'), 'nope'))).toEqual(['window:@1', 'diff'])
  })

  it('moves to the neighbouring tab when the active one closes', () => {
    const layout = closeTab(openTab(openTab(attached(), view('docs')), view('diff')), 'p0', 'diff')
    expect(panelsOf(layout)[0].activeTab).toBe('docs')
  })

  it('lists every tab once, in order, across panels', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    const layout = placeTab(openTab(split, view('docs'), panelId(split, 0)), view('tasks'), panelId(split, 1))
    expect(orderedTabs(layout).map(({ tab }) => tab.id)).toEqual(['window:@1', 'docs', 'window:@2', 'tasks'])
  })

  it('follows a tab as it opens and closes', () => {
    const opened = openTab(attached(), view('diff'))
    expect(orderedTabs(opened)).toHaveLength(2)
    expect(orderedTabs(closeTab(opened, 'p0', 'diff'))).toHaveLength(1)
  })

  it('moves any tab when a row drops beside another', () => {
    const layout = openTab(attached('@1', '@2'), view('docs'))
    expect(ids(moveTabBeside(layout, 'window:@2', 'window:@1', 'before'))).toEqual(['window:@2', 'window:@1', 'docs'])
    expect(ids(moveTabBeside(layout, 'docs', 'window:@1', 'before'))).toEqual(['docs', 'window:@1', 'window:@2'])
    expect(ids(moveTabBeside(layout, 'window:@1', 'docs', 'after'))).toEqual(['window:@2', 'docs', 'window:@1'])
  })

  it('carries the params of a tab and replaces them in place', () => {
    const layout = openTab(attached(), { id: 'docs', kind: 'docs', params: { scope: 'a' } })
    expect(tabById(layout, 'docs')?.params).toEqual({ scope: 'a' })
    const changed = withTabParams(layout, 'docs', { scope: 'b' })
    expect(tabById(changed, 'docs')?.params).toEqual({ scope: 'b' })
    expect(ids(changed)).toEqual(ids(layout))
    expect(withTabParams(layout, 'nope', {})).toBe(layout)
  })

  it('reports what each panel shows, and nothing while it asks', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    expect([...shownTabs(split).entries()].map(([panel, tab]) => [panel, tab.id])).toEqual([
      ['p0', 'window:@1'],
      [panelId(split, 1), 'window:@2'],
    ])
    expect(shownTabs(splitPanel(attached(), 'p0', 'row')).size).toBe(1)
  })

  it('focuses a panel that exists and ignores one that does not', () => {
    const split = splitPanel(attached(), 'p0', 'row')
    expect(focusPanel(split, 'p0').focusedPanel).toBe('p0')
    expect(focusPanel(split, 'nope')).toBe(split)
  })
})

describe('addTabQuietly', () => {
  it('adds the tab without taking the screen or the keyboard', () => {
    const layout = addTabQuietly(attached('@1'), view('docs'))
    expect(ids(layout)).toEqual(['window:@1', 'docs'])
    expect(focusedTab(layout)).toBe('window:@1')
  })

  it('leaves a tab that is already open alone', () => {
    const layout = openTab(attached('@1'), view('docs'))
    expect(addTabQuietly(layout, view('docs'))).toBe(layout)
  })

  it('joins the last panel, behind its picker', () => {
    const layout = addTabQuietly(splitPanel(attached('@1'), 'p0', 'row'), view('docs'))
    expect(ids(layout, 1)).toEqual(['docs'])
    expect(panelsOf(layout)[1].picking).toBe(true)
  })
})

describe('addTabBeside', () => {
  it('splits the focused panel and shows the tab in the new one', () => {
    const layout = addTabBeside(attached('@1'), view('docs'))
    expect(panelsOf(layout)).toHaveLength(2)
    expect(splitBoxes(layout).map(({ split }) => split.axis)).toEqual(['row'])
    expect(ids(layout, 0)).toEqual(['window:@1'])
    expect(ids(layout, 1)).toEqual(['docs'])
    expect(panelsOf(layout)[1].activeTab).toBe('docs')
    expect(panelsOf(layout)[1].picking).toBe(false)
    expect(shownTabs(layout).get(panelId(layout, 1))?.id).toBe('docs')
  })

  it('keeps the keyboard where the user had it', () => {
    const layout = addTabBeside(attached('@1'), view('docs'))
    expect(layout.focusedPanel).toBe('p0')
    expect(focusedTab(layout)).toBe('window:@1')
  })

  it('leaves a tab that is already open alone', () => {
    const layout = openTab(attached('@1'), view('docs'))
    expect(addTabBeside(layout, view('docs'))).toBe(layout)
  })

  it('does nothing the second time', () => {
    const once = addTabBeside(attached('@1'), view('docs'))
    expect(addTabBeside(once, view('docs'))).toBe(once)
  })
})

describe('nested splits', () => {
  it('splits a panel and asks what goes in the new one', () => {
    const layout = splitPanel(attached(), 'p0', 'row')
    expect(panelsOf(layout)).toHaveLength(2)
    expect(panelsOf(layout)[1].picking).toBe(true)
    expect(layout.focusedPanel).toBe(panelId(layout, 1))
  })

  it('splits within a split, as deep as asked, with its own direction', () => {
    const one = splitPanel(attached(), 'p0', 'row')
    const two = splitPanel(one, panelId(one, 1), 'column')
    const three = splitPanel(two, 'p0', 'column')
    expect(panelsOf(three)).toHaveLength(4)
    expect(splitBoxes(three).map(({ split }) => split.axis)).toEqual(['row', 'column', 'column'])
    expect(new Set(panelsOf(three).map((panel) => panel.id)).size).toBe(4)
  })

  it('places panels in boxes that fill the layout without overlap', () => {
    const one = splitPanel(attached(), 'p0', 'row')
    const two = splitPanel(one, panelId(one, 1), 'column')
    expect([...panelBoxes(two).values()]).toEqual([
      { x: 0, y: 0, width: 0.5, height: 1 },
      { x: 0.5, y: 0, width: 0.5, height: 0.5 },
      { x: 0.5, y: 0.5, width: 0.5, height: 0.5 },
    ])
  })

  it('places a nested boundary inside the box it divides', () => {
    const one = splitPanel(attached(), 'p0', 'row')
    const two = splitPanel(one, panelId(one, 1), 'column')
    expect(splitBoxes(two)[1].box).toEqual({ x: 0.5, y: 0, width: 0.5, height: 1 })
  })

  it('collapses only the split that lost a panel', () => {
    const one = splitPanel(attached('@1', '@2', '@3'), 'p0', 'row')
    const two = splitPanel(placeTab(one, win('@2'), panelId(one, 1)), panelId(one, 1), 'column')
    const three = placeTab(two, win('@3'), panelId(two, 2))
    const closed = closeTab(placeTab(three, view('docs'), panelId(three, 2)), panelId(three, 2), 'docs')
    expect(panelsOf(closed)).toHaveLength(3)
    const gone = syncTabs(three, 'window', ['@1', '@2'], win)
    expect(panelsOf(gone)).toHaveLength(2)
    expect(splitBoxes(gone)).toHaveLength(1)
    expect(splitBoxes(gone)[0].split.axis).toBe('row')
  })

  it('closes a panel by merging its tabs into its neighbour', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    const closed = closePanel(layout, panelId(layout, 1))
    expect(panelsOf(closed)).toHaveLength(1)
    expect(ids(closed)).toEqual(['window:@1', 'window:@2'])
    expect(closed.focusedPanel).toBe(panelId(layout, 0))
  })

  it('cancels a picker without opening another', () => {
    const lone = closeTab(splitPanel(openTab(newPanelLayout(), view('docs')), 'p0', 'row'), 'p0', 'docs')
    expect(panelsOf(lone)).toHaveLength(1)
    const cancelled = closePanel(lone, panelId(lone))
    expect(panelsOf(cancelled)).toHaveLength(1)
    expect(panelsOf(cancelled)[0].picking).toBe(false)
  })

  it('moves a tab out of a panel when the picker names it', () => {
    const split = splitPanel(openTab(attached(), view('diff')), 'p0', 'row')
    const layout = placeTab(split, view('diff'), panelId(split, 1))
    expect(ids(layout)).toEqual(['window:@1'])
    expect(panelsOf(layout)[0].activeTab).toBe('window:@1')
    expect(ids(layout, 1)).toEqual(['diff'])
    expect(panelsOf(layout)[1].picking).toBe(false)
  })

  it('ignores a place into a panel that is not there', () => {
    const layout = attached()
    expect(placeTab(layout, view('diff'), 'nope')).toBe(layout)
  })
})

describe('moveTab', () => {
  const three = () => openTab(openTab(openTab(attached(), view('docs')), view('diff')), view('tasks'))

  it('reorders within a row', () => {
    expect(ids(moveTab(three(), 'tasks', 'p0', 1))).toEqual(['window:@1', 'tasks', 'docs', 'diff'])
    expect(ids(moveTab(three(), 'docs', 'p0', 4))).toEqual(['window:@1', 'diff', 'tasks', 'docs'])
  })

  it('reads the gap before the move, so dropping beside itself changes nothing', () => {
    expect(ids(moveTab(three(), 'docs', 'p0', 2))).toEqual(ids(three()))
    expect(ids(moveTab(three(), 'docs', 'p0', 1))).toEqual(ids(three()))
  })

  it('moves a tab between panels and focuses it there', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    const layout = moveTab(openTab(split, view('docs'), 'p0'), 'docs', panelId(split, 1), 0)
    expect(ids(layout)).toEqual(['window:@1'])
    expect(ids(layout, 1)).toEqual(['docs', 'window:@2'])
    expect(layout.focusedPanel).toBe(panelId(split, 1))
    expect(focusedTab(layout)).toBe('docs')
  })

  it('closes the panel its last tab left, on either side', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    const layout = moveTab(split, 'window:@1', panelId(split, 1), 0)
    expect(panelsOf(layout)).toHaveLength(1)
    expect(ids(layout)).toEqual(['window:@1', 'window:@2'])
  })

  it('fills a panel that was asking', () => {
    const split = splitPanel(openTab(attached(), view('diff')), 'p0', 'row')
    const layout = moveTab(split, 'diff', panelId(split, 1), 0)
    expect(panelsOf(layout)[1].picking).toBe(false)
    expect(ids(layout, 1)).toEqual(['diff'])
  })

  it('ignores a tab or a panel that is not there', () => {
    const layout = three()
    expect(moveTab(layout, 'nope', 'p0', 1)).toBe(layout)
    expect(moveTab(layout, 'docs', 'nope', 0)).toBe(layout)
  })
})

describe('resizing and turning', () => {
  const split = () => splitPanel(attached(), 'p0', 'row')
  const splitId = (layout: PanelLayout) => splitBoxes(layout)[0].split.id

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
    const two = splitPanel(one, panelId(one, 1), 'column')
    const [outer, inner] = splitBoxes(two)
    const sized = resizeSplit(two, inner.split.id, 0.3)
    expect(splitBoxes(sized)[0].split.ratio).toBe(outer.split.ratio)
    expect(splitBoxes(sized)[1].split.ratio).toBe(0.3)
  })

  it('turns one split with its contents in place', () => {
    const open = placeTab(split(), view('diff'), 'p1')
    const turned = turnSplit(open, splitId(open))
    expect(splitBoxes(turned)[0].split.axis).toBe('column')
    expect(ids(turned, 1)).toEqual(['diff'])
  })
})

describe('isPanelLayout', () => {
  it('accepts a real layout and refuses what storage might hold instead', () => {
    expect(isPanelLayout(splitPanel(attached(), 'p0', 'row'))).toBe(true)
    expect(isPanelLayout(null)).toBe(false)
    expect(isPanelLayout({ panels: [] })).toBe(false)
    expect(isPanelLayout({ root: { kind: 'split', ratio: 0.5, axis: 'row' }, focusedPanel: 'p0', nextId: 1 })).toBe(
      false,
    )
  })

  it('refuses a tab without a kind, and a layout from before panels had a name', () => {
    const tabless = { root: { kind: 'panel', id: 'p0', tabs: [{ id: 'docs' }], activeTab: null, picking: false } }
    expect(isPanelLayout({ ...tabless, focusedPanel: 'p0', nextId: 1 })).toBe(false)
    const old = { root: { kind: 'pane', id: 'p0', tabs: [], activeTab: null, picking: false }, focusedPane: 'p0' }
    expect(isPanelLayout({ ...old, nextId: 1 })).toBe(false)
  })
})
