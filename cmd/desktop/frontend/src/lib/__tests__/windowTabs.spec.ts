import { describe, expect, it } from 'vitest'
import { closePanel, newPanelLayout, openTab, panelsOf, selectTab, splitPanel, type PanelLayout } from '../panelLayout'
import {
  focusedWindow,
  inTabOrder,
  offscreenWindows,
  placeWindow,
  reconcileWindows,
  revealWindow,
  terminalPanel,
  windowIdOf,
  windowIdOfTab,
  windowOrder,
  windowTab,
  windowsOnScreen,
} from '../windowTabs'

const docs = { id: 'docs', kind: 'docs' }

function attached(...windows: string[]): PanelLayout {
  const list = windows.length ? windows : ['@1']
  return reconcileWindows(newPanelLayout(), list, { active: list[0] })
}

const ids = (layout: PanelLayout, panel = 0) => panelsOf(layout)[panel].tabs.map((tab) => tab.id)
const panelId = (layout: PanelLayout, panel = 0) => panelsOf(layout)[panel].id

function splitWith(layout: PanelLayout, windowId: string): PanelLayout {
  const split = splitPanel(layout, panelId(layout), 'row')
  return placeWindow(split, windowId, panelId(split, 1))
}

describe('window tabs', () => {
  it('names the window a tab shows, and nothing for a view', () => {
    expect(windowIdOf(windowTab('@1'))).toBe('@1')
    expect(windowIdOf(docs)).toBeUndefined()
    expect(windowIdOf(undefined)).toBeUndefined()
    expect(windowIdOfTab(attached('@1'), 'window:@1')).toBe('@1')
    expect(windowIdOfTab(attached('@1'), 'docs')).toBeUndefined()
  })

  it('gives every window a tab, in order, and reads the order back', () => {
    const layout = attached('@1', '@2', '@3')
    expect(ids(layout)).toEqual(['window:@1', 'window:@2', 'window:@3'])
    expect(windowOrder(layout)).toEqual(['@1', '@2', '@3'])
  })

  it('shows the active window first', () => {
    expect(focusedWindow(reconcileWindows(newPanelLayout(), ['@1', '@2'], { active: '@2' }))).toBe('@2')
  })

  it('leaves views alone when a session has no windows left', () => {
    expect(ids(reconcileWindows(openTab(attached('@1'), docs), []))).toEqual(['docs'])
  })

  it('tells the focused window from a view in front of it', () => {
    const layout = openTab(attached('@1'), docs)
    expect(focusedWindow(layout)).toBe('')
    expect(focusedWindow(selectTab(layout, 'p0', 'window:@1'))).toBe('@1')
  })

  it('reveals a window where its tab lives', () => {
    const start = splitWith(attached('@1', '@2'), '@2')
    const revealed = revealWindow(selectTab(start, panelId(start), 'window:@1'), '@2')
    expect(revealed.focusedPanel).toBe(panelId(start, 1))
    expect(focusedWindow(revealed)).toBe('@2')
  })

  it('makes no tab for a window that has not landed, so tree order holds', () => {
    const layout = newPanelLayout()
    expect(revealWindow(layout, '@2')).toBe(layout)
  })

  it('brings the terminal forward from behind a view', () => {
    expect(panelsOf(revealWindow(openTab(attached('@1'), docs), '@1'))[0].activeTab).toBe('window:@1')
  })

  it('moves a window tab when the picker names it, so it stays one tab', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(ids(layout)).toEqual(['window:@1'])
    expect(ids(placeWindow(layout, '@2', panelId(layout, 1)), 1)).toEqual(['window:@2'])
  })

  it('offers only windows behind another tab', () => {
    const layout = splitPanel(attached('@1', '@2', '@3'), 'p0', 'row')
    expect(offscreenWindows(layout, ['@1', '@2', '@3'])).toEqual(['@2', '@3'])
  })

  it('reports which panel shows each window, and nothing behind a view', () => {
    const layout = splitWith(attached('@1', '@2'), '@2')
    expect(windowsOnScreen(layout).get('@2')).toBe(panelId(layout, 1))
    expect(windowsOnScreen(openTab(layout, docs, 'p0')).has('@1')).toBe(false)
  })

  it('names the panel where a terminal state shows', () => {
    expect(terminalPanel(newPanelLayout())).toBe('p0')
    expect(terminalPanel(openTab(attached(), docs))).toBe('')
    const split = splitPanel(openTab(attached(), docs), 'p0', 'row')
    expect(terminalPanel(split)).toBe('')
    expect(terminalPanel(placeWindow(split, '@1', panelId(split, 1)))).toBe(panelId(split, 1))
  })

  it('orders windows as the tabs show them, not as tmux does', () => {
    const split = splitPanel(attached('@1', '@2'), 'p0', 'row')
    const layout = closePanel(placeWindow(split, '@2', panelId(split, 1)), 'p0')
    expect(windowOrder(layout)).toEqual(['@2', '@1'])
    const tmuxOrder = [{ windowId: '@1' }, { windowId: '@2' }, { windowId: '@9' }]
    expect(inTabOrder(tmuxOrder, layout).map((window) => window.windowId)).toEqual(['@2', '@1', '@9'])
  })

  it('adds a new window to the panel that asked, and answers its picker', () => {
    const asking = splitPanel(attached('@1'), 'p0', 'row')
    const layout = reconcileWindows(asking, ['@1', '@2'], { active: '@1', target: asking.focusedPanel })
    const panel = panelsOf(layout).find((candidate) => candidate.id === asking.focusedPanel)!
    expect(panel.picking).toBe(false)
    expect(panel.tabs.map((tab) => tab.id)).toEqual(['window:@2'])
  })
})
