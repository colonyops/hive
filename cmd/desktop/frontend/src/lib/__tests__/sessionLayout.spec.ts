import { describe, expect, it } from 'vitest'
import {
  MAX_SPLIT_RATIO,
  MIN_SPLIT_RATIO,
  addViewQuietly,
  cancelPicker,
  chooseSplit,
  closeTab,
  focusedTab,
  focusedWindow,
  isSessionLayout,
  moveTab,
  moveTabBeside,
  newSessionLayout,
  offscreenWindows,
  openView,
  orderedTabs,
  placeView,
  placeWindow,
  reconcileWindows,
  resizeSplit,
  revealWindow,
  selectTab,
  terminalPane,
  toggleSplit,
  windowOrder,
  windowsOnScreen,
  type SessionLayout,
} from '../sessionLayout'

function attached(...windows: string[]): SessionLayout {
  const ids = windows.length ? windows : ['@1']
  return reconcileWindows(newSessionLayout(), ids, { active: ids[0] })
}

const ids = (layout: SessionLayout, pane = 0) => layout.panes[pane].tabs.map((tab) => tab.id)

function splitWith(layout: SessionLayout, windowId: string): SessionLayout {
  return placeWindow(toggleSplit(layout), windowId, 1)
}

describe('a tab per window', () => {
  it('gives every window a tab, in order', () => {
    const layout = attached('@1', '@2', '@3')
    expect(ids(layout)).toEqual(['window:@1', 'window:@2', 'window:@3'])
    expect(windowOrder(layout)).toEqual(['@1', '@2', '@3'])
  })

  it('shows the active window first', () => {
    const layout = reconcileWindows(newSessionLayout(), ['@1', '@2'], { active: '@2' })
    expect(focusedWindow(layout)).toBe('@2')
  })

  it('adds a tab for a new window in the focused pane, or the pane that asked', () => {
    const split = splitWith(attached('@1', '@2'), '@2')
    expect(ids(reconcileWindows(split, ['@1', '@2', '@3']), 1)).toEqual(['window:@2', 'window:@3'])
    expect(ids(reconcileWindows(split, ['@1', '@2', '@3'], { target: 0 }))).toEqual(['window:@1', 'window:@3'])
  })

  it('takes the tab away with its window, and moves to a neighbour', () => {
    const layout = reconcileWindows(selectTab(attached('@1', '@2'), 0, 'window:@2'), ['@1'])
    expect(ids(layout)).toEqual(['window:@1'])
    expect(layout.panes[0].activeTab).toBe('window:@1')
  })

  it('closes a pane whose only window went', () => {
    const layout = reconcileWindows(splitWith(attached('@1', '@2'), '@2'), ['@1'])
    expect(layout.panes).toHaveLength(1)
  })

  it('leaves views alone when a session has no windows left', () => {
    const layout = reconcileWindows(openView(attached('@1'), 'docs'), [])
    expect(ids(layout)).toEqual(['docs'])
  })

  it('returns the same layout when nothing changed, so a poll costs no write', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(reconcileWindows(layout, ['@1', '@2'])).toBe(layout)
  })

  it('leaves a pane that is still asking alone', () => {
    const layout = toggleSplit(attached('@1'))
    expect(reconcileWindows(layout, ['@1'])).toBe(layout)
  })

  it('reveals a window where its tab lives', () => {
    const layout = selectTab(splitWith(attached('@1', '@2'), '@2'), 0, 'window:@1')
    const revealed = revealWindow(layout, '@2')
    expect(revealed.focusedPane).toBe(1)
    expect(focusedWindow(revealed)).toBe('@2')
  })

  it('makes no tab for a window that has not landed, so tree order holds', () => {
    const layout = newSessionLayout()
    expect(revealWindow(layout, '@2')).toBe(layout)
  })

  it('brings the terminal forward from behind a view', () => {
    const layout = revealWindow(openView(attached('@1'), 'docs'), '@1')
    expect(layout.panes[0].activeTab).toBe('window:@1')
  })
})

describe('the tree mirrors the tabs', () => {
  it('lists every tab once, in tab order, across panes', () => {
    const layout = placeView(openView(splitWith(attached('@1', '@2'), '@2'), 'docs', 0), 'tasks', 1)
    expect(orderedTabs(layout).map(({ pane, tab }) => [pane, tab.id])).toEqual([
      [0, 'window:@1'],
      [0, 'docs'],
      [1, 'window:@2'],
      [1, 'tasks'],
    ])
  })

  it('follows a view as it opens and closes', () => {
    const opened = openView(attached(), 'diff')
    expect(orderedTabs(opened)).toHaveLength(2)
    expect(orderedTabs(closeTab(opened, 0, 'diff'))).toHaveLength(1)
  })

  it('follows a drag in the tabs', () => {
    expect(windowOrder(moveTab(attached('@1', '@2', '@3'), 'window:@3', 0, 0))).toEqual(['@3', '@1', '@2'])
  })

  it('moves a tab when a tree row drops beside another', () => {
    const layout = attached('@1', '@2', '@3')
    expect(windowOrder(moveTabBeside(layout, 'window:@3', 'window:@1', 'before'))).toEqual(['@3', '@1', '@2'])
    expect(windowOrder(moveTabBeside(layout, 'window:@1', 'window:@2', 'after'))).toEqual(['@2', '@1', '@3'])
  })
})

describe('tabs', () => {
  it('focuses a view that is already open instead of opening it twice', () => {
    const opened = selectTab(openView(attached(), 'diff'), 0, 'window:@1')
    const again = openView(opened, 'diff')
    expect(ids(again)).toEqual(['window:@1', 'diff'])
    expect(again.panes[0].activeTab).toBe('diff')
  })

  it('focuses a view open in the other pane rather than adding it to this one', () => {
    const layout = placeView(toggleSplit(attached()), 'diff', 1)
    const again = openView(layout, 'diff', 0)
    expect(again.focusedPane).toBe(1)
    expect(again.panes[0].tabs).toHaveLength(1)
  })

  it('moves to the neighbouring tab when the active one closes', () => {
    const layout = openView(openView(attached(), 'docs'), 'diff')
    expect(closeTab(layout, 0, 'diff').panes[0].activeTab).toBe('docs')
  })
})

describe('split', () => {
  it('opens a second pane that asks what goes there', () => {
    const layout = toggleSplit(attached())
    expect(layout.panes).toHaveLength(2)
    expect(layout.panes[1].picking).toBe(true)
    expect(layout.focusedPane).toBe(1)
  })

  it('allows two panes at most: the splitter closes the second', () => {
    expect(toggleSplit(toggleSplit(attached())).panes).toHaveLength(1)
  })

  it('stops asking once a choice lands', () => {
    const layout = placeView(toggleSplit(attached()), 'tasks', 1)
    expect(layout.panes[1].picking).toBe(false)
    expect(layout.panes[1].activeTab).toBe('tasks')
  })

  it('moves a view out of the first pane when the picker names it', () => {
    const layout = placeView(toggleSplit(openView(attached(), 'diff')), 'diff', 1)
    expect(ids(layout)).toEqual(['window:@1'])
    expect(layout.panes[0].activeTab).toBe('window:@1')
    expect(ids(layout, 1)).toEqual(['diff'])
  })

  it('moves a window tab when the picker names it, so it stays one tab', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(ids(layout)).toEqual(['window:@1'])
    expect(ids(layout, 1)).toEqual(['window:@2'])
    expect(placeWindow(layout, '@2', 1).panes[1].tabs).toHaveLength(1)
  })

  it('offers only windows behind another tab', () => {
    const layout = attached('@1', '@2', '@3')
    expect(offscreenWindows(toggleSplit(layout), ['@1', '@2', '@3'])).toEqual(['@2', '@3'])
  })

  it('closes the second pane with its last tab', () => {
    const closed = closeTab(placeView(toggleSplit(attached()), 'diff', 1), 1, 'diff')
    expect(closed.panes).toHaveLength(1)
    expect(closed.focusedPane).toBe(0)
  })

  it('closes the first pane when its last tab leaves, and the second takes its place', () => {
    const layout = moveTab(splitWith(attached('@1', '@2'), '@2'), 'window:@1', 1, 0)
    expect(layout.panes).toHaveLength(1)
    expect(ids(layout)).toEqual(['window:@1', 'window:@2'])
    expect(layout.focusedPane).toBe(0)
  })

  it('keeps every tab when the splitter merges, windows included', () => {
    const layout = placeView(splitWith(attached('@1', '@2'), '@2'), 'diff', 1)
    const merged = toggleSplit(layout)
    expect(merged.panes).toHaveLength(1)
    expect(ids(merged)).toEqual(['window:@1', 'window:@2', 'diff'])
  })

  it('reports what each pane shows, and nothing behind a view', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(windowsOnScreen(layout)).toEqual(
      new Map([
        ['@1', 0],
        ['@2', 1],
      ]),
    )
    expect(windowsOnScreen(openView(layout, 'docs', 0)).has('@1')).toBe(false)
  })

  it('names the pane where a terminal state shows', () => {
    expect(terminalPane(newSessionLayout())).toBe(0)
    expect(terminalPane(openView(attached(), 'docs'))).toBe(-1)
    expect(terminalPane(placeWindow(toggleSplit(openView(attached('@1', '@2'), 'docs')), '@2', 1))).toBe(1)
  })
})

describe('addViewQuietly', () => {
  it('adds the tab without taking the screen or the keyboard', () => {
    const layout = addViewQuietly(attached('@1'), 'docs')
    expect(ids(layout)).toEqual(['window:@1', 'docs'])
    expect(layout.panes[0].activeTab).toBe('window:@1')
    expect(focusedWindow(layout)).toBe('@1')
  })

  it('leaves a tab that is already open alone', () => {
    const layout = openView(attached('@1'), 'docs')
    expect(addViewQuietly(layout, 'docs')).toBe(layout)
  })

  it('joins the second pane of a split, and fills one that was asking', () => {
    const layout = addViewQuietly(toggleSplit(attached('@1')), 'docs')
    expect(ids(layout, 1)).toEqual(['docs'])
    expect(layout.panes[1].picking).toBe(false)
  })
})

describe('cancelPicker', () => {
  it('closes the second pane', () => {
    expect(cancelPicker(toggleSplit(attached())).panes).toHaveLength(1)
  })

  it('never opens another picker when the picker is the only pane left', () => {
    const lone = closeTab(toggleSplit(openView(newSessionLayout(), 'docs')), 0, 'docs')
    expect(lone.panes).toHaveLength(1)
    const cancelled = cancelPicker(lone)
    expect(cancelled.panes).toHaveLength(1)
    expect(cancelled.panes[0].picking).toBe(false)
  })
})

describe('moveTab', () => {
  const three = () => openView(openView(openView(attached(), 'docs'), 'diff'), 'tasks')

  it('reorders within a row', () => {
    expect(ids(moveTab(three(), 'tasks', 0, 1))).toEqual(['window:@1', 'tasks', 'docs', 'diff'])
    expect(ids(moveTab(three(), 'docs', 0, 4))).toEqual(['window:@1', 'diff', 'tasks', 'docs'])
  })

  it('reads the gap before the move, so dropping beside itself changes nothing', () => {
    expect(ids(moveTab(three(), 'docs', 0, 2))).toEqual(ids(three()))
    expect(ids(moveTab(three(), 'docs', 0, 1))).toEqual(ids(three()))
  })

  it('lets a window tab lead or follow anything', () => {
    expect(ids(moveTab(three(), 'diff', 0, 0))[0]).toBe('diff')
    expect(ids(moveTab(three(), 'window:@1', 0, 4)).at(-1)).toBe('window:@1')
  })

  it('moves a tab between panes and focuses it there', () => {
    const layout = moveTab(openView(splitWith(attached('@1', '@2'), '@2'), 'docs', 0), 'docs', 1, 0)
    expect(ids(layout)).toEqual(['window:@1'])
    expect(ids(layout, 1)).toEqual(['docs', 'window:@2'])
    expect(layout.focusedPane).toBe(1)
    expect(focusedTab(layout)).toBe('docs')
  })

  it('fills a pane that was asking', () => {
    const layout = moveTab(toggleSplit(openView(attached(), 'diff')), 'diff', 1, 0)
    expect(layout.panes[1].picking).toBe(false)
    expect(ids(layout, 1)).toEqual(['diff'])
  })

  it('ignores a tab or a pane that is not there', () => {
    const layout = three()
    expect(moveTab(layout, 'nope', 0, 1)).toBe(layout)
    expect(moveTab(layout, 'docs', 1, 0)).toBe(layout)
  })
})

describe('split direction', () => {
  it('splits the way the button says, at any width', () => {
    const layout = chooseSplit(attached(), 'column', false)
    expect(layout.panes).toHaveLength(2)
    expect(layout.splitAxis).toBe('column')
  })

  it('turns an open split without touching its contents', () => {
    const open = placeView(chooseSplit(attached(), 'row', false), 'diff', 1)
    const turned = chooseSplit(open, 'column', false)
    expect(turned.splitAxis).toBe('column')
    expect(turned.panes[1].activeTab).toBe('diff')
  })

  it('closes a split that already runs that way', () => {
    expect(chooseSplit(chooseSplit(attached(), 'row', false), 'row', false).panes).toHaveLength(1)
    expect(chooseSplit(chooseSplit(attached(), 'column', true), 'column', true).panes).toHaveLength(1)
  })

  it('reads an auto-stacked split as stacked', () => {
    const auto = toggleSplit(attached())
    expect(chooseSplit(auto, 'column', true).panes).toHaveLength(1)
    expect(chooseSplit(auto, 'row', true).splitAxis).toBe('row')
  })
})

describe('resizeSplit', () => {
  it('clamps so neither pane can vanish', () => {
    expect(resizeSplit(attached(), 0.01).splitRatio).toBe(MIN_SPLIT_RATIO)
    expect(resizeSplit(attached(), 0.99).splitRatio).toBe(MAX_SPLIT_RATIO)
    expect(resizeSplit(attached(), 0.3).splitRatio).toBe(0.3)
  })

  it('returns the same layout at the same size', () => {
    const layout = attached()
    expect(resizeSplit(layout, layout.splitRatio)).toBe(layout)
  })
})

describe('isSessionLayout', () => {
  it('accepts a real layout and refuses what storage might hold instead', () => {
    expect(isSessionLayout(toggleSplit(attached()))).toBe(true)
    expect(isSessionLayout({ ...attached(), splitRatio: 'half' })).toBe(false)
    expect(isSessionLayout(null)).toBe(false)
    expect(isSessionLayout({ panes: [] })).toBe(false)
  })
})
