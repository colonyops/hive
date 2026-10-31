import { describe, expect, it } from 'vitest'
import {
  MAX_SPLIT_RATIO,
  MIN_SPLIT_RATIO,
  TERMINAL_TAB_ID,
  chooseSplit,
  closeTab,
  focusedTab,
  moveTab,
  openViews,
  resizeSplit,
  focusedWindow,
  freeWindows,
  isSessionLayout,
  newSessionLayout,
  openView,
  placeView,
  placeWindow,
  reconcileWindows,
  revealWindow,
  selectTab,
  toggleSplit,
  windowsOnScreen,
  type SessionLayout,
} from '../sessionLayout'

function attached(windowId = '@1'): SessionLayout {
  return { ...newSessionLayout(), terminalWindow: windowId }
}

function splitWith(layout: SessionLayout, windowId: string): SessionLayout {
  return placeWindow(toggleSplit(layout), windowId, 1)
}

describe('tabs', () => {
  it('starts with a Terminal tab that cannot close', () => {
    const layout = attached()
    expect(layout.panes[0].tabs).toEqual([{ id: TERMINAL_TAB_ID, kind: 'terminal' }])
    expect(closeTab(layout, 0, TERMINAL_TAB_ID)).toBe(layout)
  })

  it('focuses a view that is already open instead of opening it twice', () => {
    const opened = selectTab(openView(attached(), 'diff'), 0, TERMINAL_TAB_ID)
    const again = openView(opened, 'diff')
    expect(again.panes[0].tabs.map((tab) => tab.id)).toEqual([TERMINAL_TAB_ID, 'diff'])
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
    expect(layout.panes[0].tabs.map((tab) => tab.id)).toEqual([TERMINAL_TAB_ID])
    expect(layout.panes[0].activeTab).toBe(TERMINAL_TAB_ID)
    expect(layout.panes[1].tabs.map((tab) => tab.id)).toEqual(['diff'])
  })

  it('closes the second pane with its last tab', () => {
    const layout = placeView(toggleSplit(attached()), 'diff', 1)
    const closed = closeTab(layout, 1, 'diff')
    expect(closed.panes).toHaveLength(1)
    expect(closed.focusedPane).toBe(0)
  })

  it('keeps views and lets go of window tabs when the splitter merges', () => {
    const layout = placeView(splitWith(attached(), '@2'), 'diff', 1)
    const merged = toggleSplit(layout)
    expect(merged.panes).toHaveLength(1)
    expect(merged.panes[0].tabs.map((tab) => tab.id)).toEqual([TERMINAL_TAB_ID, 'diff'])
  })
})

describe('windows', () => {
  it('shows a window in one place: picking the Terminal tab window focuses it', () => {
    const layout = placeWindow(toggleSplit(attached('@1')), '@1', 1)
    expect(layout.panes[1].tabs).toHaveLength(0)
    expect(layout.focusedPane).toBe(0)
  })

  it('does not add a second tab for a window already in the pane', () => {
    const layout = splitWith(attached(), '@2')
    expect(placeWindow(layout, '@2', 1).panes[1].tabs).toHaveLength(1)
  })

  it('reveals a window from the tree in the Terminal tab', () => {
    const layout = revealWindow(openView(attached('@1'), 'docs'), '@3')
    expect(layout.terminalWindow).toBe('@3')
    expect(layout.panes[0].activeTab).toBe(TERMINAL_TAB_ID)
    expect(focusedWindow(layout)).toBe('@3')
  })

  it('focuses a window from the tree where it already shows', () => {
    const layout = selectTab(splitWith(attached('@1'), '@2'), 0, TERMINAL_TAB_ID)
    const revealed = revealWindow(layout, '@2')
    expect(revealed.terminalWindow).toBe('@1')
    expect(revealed.focusedPane).toBe(1)
    expect(focusedWindow(revealed)).toBe('@2')
  })

  it('leaves the window out of the screen when its tab is hidden, without touching the Terminal tab', () => {
    const layout = closeTab(placeView(splitWith(attached('@1'), '@2'), 'docs', 1), 1, 'window:@2')
    expect([...windowsOnScreen(layout).keys()]).toEqual(['@1'])
    expect(freeWindows(layout, ['@1', '@2'])).toEqual(['@2'])
  })

  it('reports what each pane shows, and nothing behind a view', () => {
    const layout = splitWith(attached('@1'), '@2')
    expect(windowsOnScreen(layout)).toEqual(
      new Map([
        ['@1', 0],
        ['@2', 1],
      ]),
    )
    expect(windowsOnScreen(openView(layout, 'docs', 0)).has('@1')).toBe(false)
  })

  it('offers only windows with no place of their own', () => {
    expect(freeWindows(splitWith(attached('@1'), '@2'), ['@1', '@2', '@3'])).toEqual(['@3'])
  })
})

describe('reconcileWindows', () => {
  it('returns the same layout when nothing changed, so a poll costs no write', () => {
    const layout = splitWith(attached('@1'), '@2')
    expect(reconcileWindows(layout, ['@1', '@2', '@3'])).toBe(layout)
  })

  it('adopts the active window on a fresh layout', () => {
    expect(reconcileWindows(newSessionLayout(), ['@1', '@2'], '@2').terminalWindow).toBe('@2')
  })

  it('moves the Terminal tab to a free window when its window closes', () => {
    const layout = reconcileWindows(splitWith(attached('@1'), '@2'), ['@2', '@3'])
    expect(layout.terminalWindow).toBe('@3')
    expect(layout.panes[1].tabs.map((tab) => tab.id)).toEqual(['window:@2'])
  })

  it('takes the last window back from the second pane, which closes empty', () => {
    const layout = reconcileWindows(splitWith(attached('@1'), '@2'), ['@2'])
    expect(layout.terminalWindow).toBe('@2')
    expect(layout.panes).toHaveLength(1)
  })

  it('drops the tab of a window that closed, along with an emptied pane', () => {
    const layout = reconcileWindows(splitWith(attached('@1'), '@2'), ['@1'])
    expect(layout.panes).toHaveLength(1)
  })

  it('leaves a pane that is still asking alone', () => {
    const layout = toggleSplit(attached('@1'))
    expect(reconcileWindows(layout, ['@1'])).toBe(layout)
  })
})

describe('moveTab', () => {
  const ids = (layout: SessionLayout, pane = 0) => layout.panes[pane].tabs.map((tab) => tab.id)
  const three = () => openView(openView(openView(attached(), 'docs'), 'diff'), 'tasks')

  it('reorders within a row', () => {
    expect(ids(moveTab(three(), 'tasks', 0, 1))).toEqual([TERMINAL_TAB_ID, 'tasks', 'docs', 'diff'])
    expect(ids(moveTab(three(), 'docs', 0, 4))).toEqual([TERMINAL_TAB_ID, 'diff', 'tasks', 'docs'])
  })

  it('reads the gap before the move, so dropping beside itself changes nothing', () => {
    expect(ids(moveTab(three(), 'docs', 0, 2))).toEqual(ids(three()))
    expect(ids(moveTab(three(), 'docs', 0, 1))).toEqual(ids(three()))
  })

  it('pins the Terminal tab first: it does not move and nothing lands ahead of it', () => {
    const layout = three()
    expect(moveTab(layout, TERMINAL_TAB_ID, 0, 3)).toBe(layout)
    expect(ids(moveTab(layout, 'diff', 0, 0))[0]).toBe(TERMINAL_TAB_ID)
  })

  it('moves a tab between panes and focuses it there', () => {
    const layout = moveTab(openView(splitWith(attached(), '@2'), 'docs', 0), 'docs', 1, 0)
    expect(ids(layout)).toEqual([TERMINAL_TAB_ID])
    expect(ids(layout, 1)).toEqual(['docs', 'window:@2'])
    expect(layout.focusedPane).toBe(1)
    expect(focusedTab(layout)).toBe('docs')
  })

  it('moves a window tab too, and closes the second pane when that empties it', () => {
    const layout = moveTab(splitWith(attached('@1'), '@2'), 'window:@2', 0, 1)
    expect(layout.panes).toHaveLength(1)
    expect(ids(layout)).toEqual([TERMINAL_TAB_ID, 'window:@2'])
    expect(windowsOnScreen(layout).get('@2')).toBe(0)
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

describe('openViews', () => {
  it('lists Docs, Diff and Tasks with their panes, and neither terminals nor windows', () => {
    const layout = placeView(openView(splitWith(attached(), '@2'), 'docs', 0), 'tasks', 1)
    expect(openViews(layout).map(({ pane, tab }) => [pane, tab.id])).toEqual([
      [0, 'docs'],
      [1, 'tasks'],
    ])
  })

  it('follows a tab as it opens and closes', () => {
    const opened = openView(attached(), 'diff')
    expect(openViews(opened)).toHaveLength(1)
    expect(openViews(closeTab(opened, 0, 'diff'))).toHaveLength(0)
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
    expect(isSessionLayout({ panes: [{ tabs: [{ id: 'diff', kind: 'diff' }] }], terminalWindow: '' })).toBe(false)
  })
})
