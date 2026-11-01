import { effectScope, nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { focusPanel, openTab, panelsOf, selectTab, splitBoxes, splitPanel } from '../../../lib/panelLayout'
import { paneMayAutoFocus } from '../../../lib/terminalTree'
import { focusedWindow, placeWindow, windowTab } from '../../../lib/windowTabs'
import { usePanelLayouts } from '../../../stores/usePanelLayouts'
import { viewTab } from '../panelViews'
import { useSessionPanels } from '../useSessionPanels'

const KEY = 'session-1'

function fakeSource(initial: string[], active = initial[0] ?? '') {
  const windowIds = ref<string[]>(initial)
  const activeWindow = ref(active)
  const rendered: string[][] = []
  return {
    key: ref(KEY),
    windowIds,
    activeWindow,
    closed: ref(false),
    pending: ref(false),
    // As the real one does: a select makes the window active straight away.
    select: vi.fn((windowId: string) => {
      activeWindow.value = windowId
    }),
    moveWindow: vi.fn(),
    render: vi.fn((onScreen: Iterable<string>) => void rendered.push([...onScreen])),
    focus: vi.fn(),
    rendered,
  }
}

type FakeSource = ReturnType<typeof fakeSource>

function mountPanels(source: FakeSource) {
  const scope = effectScope()
  const panels = scope.run(() => useSessionPanels(source, usePanelLayouts()))!
  return { panels, scope }
}

describe('useSessionPanels', () => {
  beforeEach(() => {
    localStorage.clear()
    paneMayAutoFocus.value = true
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  describe('a window activated by an agent or another client', () => {
    it('does not take the keyboard from the terminal in front of the user', async () => {
      const source = fakeSource(['@1', '@2'])
      mountPanels(source)
      await flushPromises()
      source.focus.mockClear()

      source.activeWindow.value = '@2'
      await flushPromises()

      expect(focusedWindow(usePanelLayouts().layoutOf(KEY))).toBe('@2')
      expect(source.focus).not.toHaveBeenCalled()
    })

    it('does not replace a view the user is reading', async () => {
      const source = fakeSource(['@1', '@2'])
      mountPanels(source)
      await flushPromises()
      usePanelLayouts().apply(KEY, (layout) => openTab(layout, viewTab('diff')))
      await flushPromises()

      source.activeWindow.value = '@2'
      await flushPromises()

      expect(panelsOf(usePanelLayouts().layoutOf(KEY))[0].activeTab).toBe('diff')
    })
  })

  describe('a window the user asked for', () => {
    it('comes forward even from behind a view, in the panel that asked', async () => {
      const source = fakeSource(['@1'])
      const { panels } = mountPanels(source)
      await flushPromises()
      const layouts = usePanelLayouts()
      layouts.apply(KEY, (layout) => splitPanel(openTab(layout, viewTab('diff')), 'p0', 'row'))
      const target = panelsOf(layouts.layoutOf(KEY))[1].id

      panels.expectWindowIn(target)
      source.windowIds.value = ['@1', '@2']
      source.activeWindow.value = '@2'
      await flushPromises()

      expect(panelsOf(layouts.layoutOf(KEY))[1].activeTab).toBe('window:@2')
      expect(focusedWindow(layouts.layoutOf(KEY))).toBe('@2')
    })

    it('moves the tmux window to match when the new tab lands mid-order', async () => {
      const source = fakeSource(['@1', '@2'])
      const { panels } = mountPanels(source)
      await flushPromises()
      usePanelLayouts().apply(KEY, (layout) => {
        const split = splitPanel(layout, 'p0', 'row')
        return placeWindow(split, '@2', panelsOf(split)[1].id)
      })
      await flushPromises()

      panels.expectWindowIn('p0')
      source.windowIds.value = ['@1', '@2', '@3']
      await flushPromises()

      expect(source.moveWindow).toHaveBeenCalledWith('@3', 1)
    })

    it('moves a dragged window tab to its place among the tmux windows', async () => {
      const source = fakeSource(['@1', '@2'])
      const { panels } = mountPanels(source)
      await flushPromises()

      panels.moveTabTo('window:@2', 'p0', 0)

      expect(source.moveWindow).toHaveBeenCalledWith('@2', 0)
    })

    it('leaves tmux alone when a view tab moves', async () => {
      const source = fakeSource(['@1'])
      const { panels } = mountPanels(source)
      await flushPromises()
      usePanelLayouts().apply(KEY, (layout) => openTab(layout, viewTab('diff')))

      panels.moveTabTo('diff', 'p0', 0)

      expect(source.moveWindow).not.toHaveBeenCalled()
    })

    // Only setTimeout is faked: flushPromises itself drains through setImmediate.
    it('forgets a request that never landed, so a later window does not claim its panel', async () => {
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
      const source = fakeSource(['@1'])
      const { panels } = mountPanels(source)
      await flushPromises()
      const layouts = usePanelLayouts()
      layouts.apply(KEY, (layout) => focusPanel(splitPanel(layout, 'p0', 'row'), 'p0'))
      const asked = panelsOf(layouts.layoutOf(KEY))[1].id

      panels.expectWindowIn(asked)
      vi.advanceTimersByTime(5000)
      source.windowIds.value = ['@1', '@2']
      await flushPromises()

      const [first, second] = panelsOf(layouts.layoutOf(KEY))
      expect(first.tabs.map((tab) => tab.id)).toEqual(['window:@1', 'window:@2'])
      expect(second.picking).toBe(true)
      expect(focusedWindow(layouts.layoutOf(KEY))).toBe('@1')
      expect(source.moveWindow).not.toHaveBeenCalled()
    })
  })

  describe('a window an agent made', () => {
    const layoutNow = () => usePanelLayouts().layoutOf(KEY)

    it('joins the focused panel without being selected when it goes behind', async () => {
      const source = fakeSource(['@1'])
      const { panels } = mountPanels(source)
      await flushPromises()

      panels.placeArrivingWindow('@2', 'behind')
      source.windowIds.value = ['@1', '@2']
      await flushPromises()

      expect(panelsOf(layoutNow())).toHaveLength(1)
      expect(panelsOf(layoutNow())[0].tabs.map((tab) => tab.id)).toEqual(['window:@1', 'window:@2'])
      expect(focusedWindow(layoutNow())).toBe('@1')
    })

    it('moves a tab that is already there into a split beside the focused panel', async () => {
      const source = fakeSource(['@1', '@2'])
      const { panels } = mountPanels(source)
      await flushPromises()

      panels.placeArrivingWindow('@2', 'beside')
      await flushPromises()

      const [left, right] = panelsOf(layoutNow())
      expect(left.tabs.map((tab) => tab.id)).toEqual(['window:@1'])
      expect(right.activeTab).toBe('window:@2')
      expect(focusedWindow(layoutNow())).toBe('@1')
    })

    it('splits ahead of the tmux listing and takes the window when it lands', async () => {
      const source = fakeSource(['@1'])
      const { panels } = mountPanels(source)
      await flushPromises()
      source.focus.mockClear()

      panels.placeArrivingWindow('@2', 'beside')
      expect(panelsOf(layoutNow())[1].picking).toBe(true)
      expect(layoutNow().focusedPanel).toBe('p0')

      source.windowIds.value = ['@1', '@2']
      await flushPromises()

      const [left, right] = panelsOf(layoutNow())
      expect(left.tabs.map((tab) => tab.id)).toEqual(['window:@1'])
      expect(right.tabs.map((tab) => tab.id)).toEqual(['window:@2'])
      expect(right.picking).toBe(false)
      expect(right.activeTab).toBe('window:@2')
      expect(focusedWindow(layoutNow())).toBe('@1')
      expect(source.moveWindow).not.toHaveBeenCalled()
      expect(source.focus).not.toHaveBeenCalled()
    })

    it('leaves a later window out of the panel once the first one landed', async () => {
      const source = fakeSource(['@1'])
      const { panels } = mountPanels(source)
      await flushPromises()

      panels.placeArrivingWindow('@2', 'beside')
      source.windowIds.value = ['@1', '@2']
      await flushPromises()
      source.windowIds.value = ['@1', '@2', '@3']
      await flushPromises()

      const [left, right] = panelsOf(layoutNow())
      expect(left.tabs.map((tab) => tab.id)).toEqual(['window:@1', 'window:@3'])
      expect(right.tabs.map((tab) => tab.id)).toEqual(['window:@2'])
    })

    it('stacks the panel below the focused one when it goes below', async () => {
      const source = fakeSource(['@1'])
      const { panels } = mountPanels(source)
      await flushPromises()

      panels.placeArrivingWindow('@2', 'below')
      source.windowIds.value = ['@1', '@2']
      await flushPromises()

      const [top, bottom] = panelsOf(layoutNow())
      expect(splitBoxes(layoutNow()).map(({ split }) => split.axis)).toEqual(['column'])
      expect(top.tabs.map((tab) => tab.id)).toEqual(['window:@1'])
      expect(bottom.tabs.map((tab) => tab.id)).toEqual(['window:@2'])
      expect(layoutNow().focusedPanel).toBe('p0')
    })

    it('moves a tab that is already there into a column split', async () => {
      const source = fakeSource(['@1', '@2'])
      const { panels } = mountPanels(source)
      await flushPromises()

      panels.placeArrivingWindow('@2', 'below')

      expect(splitBoxes(layoutNow()).map(({ split }) => split.axis)).toEqual(['column'])
      expect(panelsOf(layoutNow())[1].activeTab).toBe('window:@2')
    })

    // p0 holds @1 and p1 holds @3, with the keyboard in p1.
    function splitWithKeyboardInSecond(): void {
      usePanelLayouts().apply(KEY, (layout) => {
        const split = splitPanel(layout, 'p0', 'row')
        return focusPanel(placeWindow(split, '@3', 'p1'), 'p1')
      })
    }

    it('splits the anchor panel, not the focused one', async () => {
      const source = fakeSource(['@1', '@3'])
      const { panels } = mountPanels(source)
      await flushPromises()
      splitWithKeyboardInSecond()

      panels.placeArrivingWindow('@2', 'beside', 'p0')
      source.windowIds.value = ['@1', '@3', '@2']
      await flushPromises()

      expect(layoutNow().focusedPanel).toBe('p1')
      const byId = new Map(panelsOf(layoutNow()).map((panel) => [panel.id, panel.tabs.map((tab) => tab.id)]))
      expect(byId.get('p0')).toEqual(['window:@1'])
      expect(byId.get('p1')).toEqual(['window:@3'])
      expect(byId.get('p2')).toEqual(['window:@2'])
    })

    it('lands behind in the anchor panel when that is not the focused one', async () => {
      const source = fakeSource(['@1', '@3'])
      const { panels } = mountPanels(source)
      await flushPromises()
      splitWithKeyboardInSecond()

      panels.placeArrivingWindow('@2', 'behind', 'p0')
      source.windowIds.value = ['@1', '@3', '@2']
      await flushPromises()

      expect(panelsOf(layoutNow())[0].tabs.map((tab) => tab.id)).toEqual(['window:@1', 'window:@2'])
      expect(panelsOf(layoutNow())[1].tabs.map((tab) => tab.id)).toEqual(['window:@3'])
      expect(layoutNow().focusedPanel).toBe('p1')
    })

    it('moves a window the list announced first behind the anchor, without selecting it', async () => {
      const source = fakeSource(['@1', '@3'], '@3')
      const { panels } = mountPanels(source)
      await flushPromises()
      splitWithKeyboardInSecond()
      source.windowIds.value = ['@1', '@3', '@2']
      await flushPromises()
      expect(panelsOf(layoutNow())[1].tabs.map((tab) => tab.id)).toEqual(['window:@3', 'window:@2'])

      panels.placeArrivingWindow('@2', 'behind', 'p0')
      await flushPromises()

      expect(panelsOf(layoutNow())[0].tabs.map((tab) => tab.id)).toEqual(['window:@1', 'window:@2'])
      expect(panelsOf(layoutNow())[1].tabs.map((tab) => tab.id)).toEqual(['window:@3'])
      expect(panelsOf(layoutNow())[1].activeTab).toBe('window:@3')
      expect(layoutNow().focusedPanel).toBe('p1')
      expect(focusedWindow(layoutNow())).toBe('@3')
    })

    it('leaves a window alone that is already in the anchor panel', async () => {
      const source = fakeSource(['@1', '@2'])
      const { panels } = mountPanels(source)
      await flushPromises()
      const before = layoutNow()

      panels.placeArrivingWindow('@1', 'behind', 'p0')

      expect(layoutNow()).toBe(before)
    })
  })

  describe('layout to tmux', () => {
    it('selects the window a click focused', async () => {
      const source = fakeSource(['@1', '@2'])
      mountPanels(source)
      await flushPromises()

      usePanelLayouts().apply(KEY, (layout) => selectTab(layout, 'p0', 'window:@2'))
      await flushPromises()

      expect(source.select).toHaveBeenCalledWith('@2')
    })

    it('puts the keyboard back when a click returns to the active window from a view', async () => {
      const source = fakeSource(['@1'])
      mountPanels(source)
      await flushPromises()
      const layouts = usePanelLayouts()
      layouts.apply(KEY, (layout) => openTab(layout, viewTab('docs')))
      await flushPromises()
      source.focus.mockClear()

      layouts.apply(KEY, (layout) => selectTab(layout, 'p0', 'window:@1'))
      await flushPromises()
      await nextTick()

      expect(source.focus).toHaveBeenCalled()
    })

    it('tells tmux which window a restored layout shows, once a cold attach lands', async () => {
      usePanelLayouts().apply(KEY, (layout) => ({
        ...layout,
        root: {
          ...panelsOf(layout)[0],
          tabs: [windowTab('@2'), windowTab('@1')],
          activeTab: 'window:@2',
        },
      }))
      const source = fakeSource([])
      source.pending.value = true
      mountPanels(source)
      await flushPromises()

      source.windowIds.value = ['@1', '@2']
      source.activeWindow.value = '@1'
      source.pending.value = false
      await flushPromises()

      expect(source.select).toHaveBeenCalledWith('@2')
    })

    it('asks for the renderer of every window on screen, not only the active one', async () => {
      const source = fakeSource(['@1', '@2'])
      mountPanels(source)
      await flushPromises()
      usePanelLayouts().apply(KEY, (layout) => {
        const split = splitPanel(layout, 'p0', 'row')
        return placeWindow(split, '@2', panelsOf(split)[1].id)
      })
      await flushPromises()

      expect(source.rendered.at(-1)).toEqual(expect.arrayContaining(['@1', '@2']))
    })
  })

  describe('what ends a layout', () => {
    const withView = async (source: FakeSource) => {
      mountPanels(source)
      await flushPromises()
      usePanelLayouts().apply(KEY, (layout) => openTab(layout, viewTab('diff')))
      await flushPromises()
    }
    const tabIds = () => panelsOf(usePanelLayouts().layoutOf(KEY))[0].tabs.map((tab) => tab.id)

    it('closes every tab when the source is closed', async () => {
      const source = fakeSource(['@1'])
      await withView(source)

      source.windowIds.value = []
      source.closed.value = true
      await flushPromises()

      expect(tabIds()).toEqual([])
    })

    it('keeps every tab while the window list is pending', async () => {
      const source = fakeSource(['@1'])
      await withView(source)

      source.windowIds.value = []
      source.pending.value = true
      await flushPromises()

      expect(tabIds()).toEqual(['window:@1', 'diff'])
    })

    it('drops the window tabs and keeps the views when the list is empty and settled', async () => {
      const source = fakeSource(['@1'])
      await withView(source)

      source.windowIds.value = []
      await flushPromises()

      expect(tabIds()).toEqual(['diff'])
    })
  })
})
