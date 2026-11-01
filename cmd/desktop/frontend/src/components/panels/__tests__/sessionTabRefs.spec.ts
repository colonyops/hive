import { describe, expect, it } from 'vitest'
import {
  focusPanel,
  newPanelLayout,
  openTab,
  panelsOf,
  placeTab,
  splitBoxes,
  splitPanel,
  type PanelLayout,
} from '../../../lib/panelLayout'
import { windowTab } from '../../../lib/windowTabs'
import { viewTab } from '../../terminal/panelViews'
import {
  anchorPanelOf,
  closePanelRule,
  focusRule,
  moveRule,
  openViewRule,
  resizeRule,
  tabIdOf,
  turnRule,
} from '../sessionTabRefs'
import type { TabRef } from '../useSessionLayout'

describe('tabIdOf', () => {
  it('names a view by its tab id, with drive as docs', () => {
    expect(tabIdOf({ view: 'drive', window: '' })).toBe('docs')
    expect(tabIdOf({ view: 'diff', window: '' })).toBe('diff')
    expect(tabIdOf({ view: 'tasks', window: '' })).toBe('tasks')
  })

  it('names a terminal by its window', () => {
    expect(tabIdOf({ view: 'terminal', window: '@3' })).toBe('window:@3')
    expect(tabIdOf({ view: 'terminal', window: '@3' }, '@9')).toBe('window:@3')
  })

  it('names the host window for a terminal with none, when the host has one', () => {
    expect(tabIdOf({ view: 'terminal', window: '' }, '@9')).toBe('window:@9')
  })

  it('names nothing for a terminal with no window and no host window', () => {
    expect(tabIdOf({ view: 'terminal', window: '' })).toBeUndefined()
    expect(tabIdOf({ view: 'terminal', window: '' }, '')).toBeUndefined()
  })
})

describe('layout rules over refs', () => {
  const terminal = (window: string): TabRef => ({ view: 'terminal', window })
  const drive: TabRef = { view: 'drive', window: '' }
  const tabIds = (layout: PanelLayout) => panelsOf(layout).map((panel) => panel.tabs.map((tab) => tab.id))

  function withWindows(...windows: string[]): PanelLayout {
    return windows.reduce((layout, id) => openTab(layout, windowTab(id)), newPanelLayout())
  }

  describe('anchorPanelOf', () => {
    it('is the anchor tab panel, else the focused one', () => {
      const split = splitPanel(withWindows('@1'), 'p0', 'row')
      const layout = focusPanel(placeTab(split, windowTab('@2'), 'p1'), 'p1')

      expect(anchorPanelOf(layout, terminal('@1'))).toBe('p0')
      expect(anchorPanelOf(layout, null)).toBe('p1')
      expect(anchorPanelOf(layout, terminal('@9'))).toBe('p1')
      expect(anchorPanelOf(layout, terminal(''))).toBe('p1')
      expect(anchorPanelOf(layout, terminal(''), '@1')).toBe('p0')
    })
  })

  describe('openViewRule', () => {
    it('stacks a view below the anchor panel', () => {
      const layout = openViewRule(withWindows('@1'), viewTab('docs'), 'below', terminal('@1'))
      expect(splitBoxes(layout).map(({ split }) => split.axis)).toEqual(['column'])
      expect(tabIds(layout)).toEqual([['window:@1'], ['docs']])
    })

    it('puts a view behind the anchor right after it', () => {
      const layout = openViewRule(withWindows('@1', '@2'), viewTab('docs'), 'behind', terminal('@1'))
      expect(tabIds(layout)).toEqual([['window:@1', 'docs', 'window:@2']])
    })

    it('puts a view behind with no anchor in the last panel', () => {
      const layout = openViewRule(withWindows('@1', '@2'), viewTab('docs'), 'behind', null)
      expect(tabIds(layout)).toEqual([['window:@1', 'window:@2', 'docs']])
    })

    it('leaves a view that is open alone when it goes behind', () => {
      const layout = openTab(withWindows('@1'), viewTab('docs'))
      expect(openViewRule(layout, viewTab('docs'), 'behind', terminal('@1'))).toBe(layout)
    })
  })

  describe('moveRule', () => {
    it('moves a tab beside the anchor, naming the chat window with an empty one', () => {
      const layout = openTab(withWindows('@1'), viewTab('docs'))
      const moved = moveRule(layout, drive, 'beside', terminal(''), '@1')
      expect(tabIds(moved)).toEqual([['window:@1'], ['docs']])
    })

    it('moves a tab behind the anchor', () => {
      const layout = openTab(withWindows('@1', '@2'), viewTab('docs'))
      expect(tabIds(moveRule(layout, drive, 'behind', terminal('@1')))).toEqual([['window:@1', 'docs', 'window:@2']])
    })

    it('does nothing for a tab or an anchor that is not open', () => {
      const layout = withWindows('@1')
      expect(moveRule(layout, drive, 'behind', null)).toBe(layout)
      expect(moveRule(openTab(layout, viewTab('docs')), drive, 'beside', terminal('@9'))).toEqual(
        openTab(layout, viewTab('docs')),
      )
    })
  })

  describe('focus, close panel, resize and turn', () => {
    const split = (): PanelLayout => {
      const layout = splitPanel(withWindows('@1'), 'p0', 'row')
      return focusPanel(placeTab(layout, windowTab('@2'), 'p1'), 'p0')
    }

    it('focuses the panel of the tab', () => {
      expect(focusRule(split(), terminal('@2')).focusedPanel).toBe('p1')
      const layout = split()
      expect(focusRule(layout, terminal('@9'))).toBe(layout)
    })

    it('closes the panel of the tab, its tabs joining the neighbour', () => {
      expect(tabIds(closePanelRule(split(), terminal('@2')))).toEqual([['window:@1', 'window:@2']])
    })

    it('resizes the split so the tab panel has the share', () => {
      const ratio = (layout: PanelLayout) => splitBoxes(layout)[0].split.ratio
      expect(ratio(resizeRule(split(), terminal('@1'), 0.7))).toBe(0.7)
      expect(ratio(resizeRule(split(), terminal('@2'), 0.7))).toBeCloseTo(0.3)
    })

    it('turns the split around the tab', () => {
      expect(splitBoxes(turnRule(split(), terminal('@1')))[0].split.axis).toBe('column')
    })

    it('does nothing for a tab that is not open', () => {
      const layout = split()
      expect(closePanelRule(layout, drive)).toBe(layout)
      expect(resizeRule(layout, drive, 0.7)).toBe(layout)
      expect(turnRule(layout, drive)).toBe(layout)
    })
  })
})
