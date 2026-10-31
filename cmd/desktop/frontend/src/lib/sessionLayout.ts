export type SessionViewKind = 'docs' | 'diff' | 'tasks'

export const TERMINAL_TAB_ID = 'terminal'

/** `auto` is side by side until the session gets too narrow, then stacked. A pick holds at any width. */
export type SplitAxis = 'auto' | 'row' | 'column'

export const MIN_SPLIT_RATIO = 0.2
export const MAX_SPLIT_RATIO = 0.8

export type SessionTab =
  | { id: string; kind: 'terminal' }
  | { id: string; kind: 'window'; windowId: string }
  | { id: string; kind: SessionViewKind }

export interface SessionPane {
  tabs: SessionTab[]
  activeTab: string | null
  /** An empty pane asking what goes in it. */
  picking: boolean
}

/**
 * What a session shows in Code. The first pane always holds the Terminal tab,
 * which shows `terminalWindow`. Ids derive from the content, so opening a view
 * twice finds the tab it already has.
 */
export interface SessionLayout {
  panes: SessionPane[]
  focusedPane: number
  terminalWindow: string
  docPath: string | null
  splitAxis: SplitAxis
  /** The first pane's share of the session. */
  splitRatio: number
}

export function newSessionLayout(): SessionLayout {
  return {
    panes: [{ tabs: [{ id: TERMINAL_TAB_ID, kind: 'terminal' }], activeTab: TERMINAL_TAB_ID, picking: false }],
    focusedPane: 0,
    terminalWindow: '',
    docPath: null,
    splitAxis: 'auto',
    splitRatio: 0.5,
  }
}

/** Storage can hold anything, so a layout that breaks the rules above starts over. */
export function isSessionLayout(value: unknown): value is SessionLayout {
  const layout = value as Partial<SessionLayout> | null | undefined
  const panes = layout?.panes
  if (!Array.isArray(panes) || panes.length < 1 || panes.length > 2) return false
  if (panes[0]?.tabs?.[0]?.kind !== 'terminal') return false
  return (
    panes.every((pane) => Array.isArray(pane?.tabs)) &&
    typeof layout?.terminalWindow === 'string' &&
    typeof layout?.splitAxis === 'string' &&
    typeof layout?.splitRatio === 'number'
  )
}

function withPane(layout: SessionLayout, index: number, change: Partial<SessionPane>): SessionLayout {
  return { ...layout, panes: layout.panes.map((pane, at) => (at === index ? { ...pane, ...change } : pane)) }
}

function paneWithTab(layout: SessionLayout, tabId: string): number {
  return layout.panes.findIndex((pane) => pane.tabs.some((tab) => tab.id === tabId))
}

function windowTabId(windowId: string): string {
  return `window:${windowId}`
}

export function focusPane(layout: SessionLayout, index: number): SessionLayout {
  if (layout.focusedPane === index || !layout.panes[index]) return layout
  return { ...layout, focusedPane: index }
}

export function selectTab(layout: SessionLayout, index: number, tabId: string): SessionLayout {
  const pane = layout.panes[index]
  if (!pane?.tabs.some((tab) => tab.id === tabId)) return layout
  return { ...withPane(layout, index, { activeTab: tabId, picking: false }), focusedPane: index }
}

function appendTab(layout: SessionLayout, index: number, tab: SessionTab): SessionLayout {
  const pane = layout.panes[index]
  return selectTab(withPane(layout, index, { tabs: [...pane.tabs, tab] }), index, tab.id)
}

/** A view that is already open anywhere is focused where it is. */
export function openView(layout: SessionLayout, kind: SessionViewKind, target = layout.focusedPane): SessionLayout {
  const existing = paneWithTab(layout, kind)
  if (existing >= 0) return selectTab(layout, existing, kind)
  return appendTab(layout, layout.panes[target] ? target : 0, { id: kind, kind })
}

/** The picker names a place, so a view open in the other pane moves here. */
export function placeView(layout: SessionLayout, kind: SessionViewKind, target: number): SessionLayout {
  const from = paneWithTab(layout, kind)
  if (from === target) return selectTab(layout, target, kind)
  const cleared = from < 0 ? layout : removeTab(layout, from, kind)
  return appendTab(cleared, target, { id: kind, kind })
}

/** A window shows in one place: one already on screen is focused rather than opened again. */
export function placeWindow(layout: SessionLayout, windowId: string, target: number): SessionLayout {
  if (windowId === layout.terminalWindow) return selectTab(layout, 0, TERMINAL_TAB_ID)
  const id = windowTabId(windowId)
  const existing = paneWithTab(layout, id)
  if (existing >= 0) return selectTab(layout, existing, id)
  return appendTab(layout, target, { id, kind: 'window', windowId })
}

function removeTab(layout: SessionLayout, index: number, tabId: string): SessionLayout {
  const pane = layout.panes[index]
  const at = pane.tabs.findIndex((tab) => tab.id === tabId)
  const tabs = pane.tabs.filter((tab) => tab.id !== tabId)
  const activeTab = pane.activeTab === tabId ? ((tabs[at] ?? tabs[at - 1])?.id ?? null) : pane.activeTab
  return withPane(layout, index, { tabs, activeTab })
}

function dropEmptySecondPane(layout: SessionLayout): SessionLayout {
  const second = layout.panes[1]
  if (!second || second.tabs.length || second.picking) return layout
  return { ...layout, panes: [layout.panes[0]], focusedPane: 0 }
}

/** The Terminal tab has no close: closing it would end the session. Closing a window's tab only hides it. */
export function closeTab(layout: SessionLayout, index: number, tabId: string): SessionLayout {
  if (tabId === TERMINAL_TAB_ID || !layout.panes[index]?.tabs.some((tab) => tab.id === tabId)) return layout
  return dropEmptySecondPane(removeTab(layout, index, tabId))
}

/** Views survive the merge into the first pane; a window's tab goes, and the window keeps running. */
export function toggleSplit(layout: SessionLayout): SessionLayout {
  const [first, second] = layout.panes
  if (!second) {
    return {
      ...layout,
      panes: [first, { tabs: [], activeTab: null, picking: true }],
      focusedPane: 1,
    }
  }
  const views = second.tabs.filter((tab) => tab.kind !== 'window')
  return { ...layout, panes: [{ ...first, tabs: [...first.tabs, ...views] }], focusedPane: 0 }
}

/** One button per direction: it splits that way, turns an existing split, or closes a split already that way. */
export function chooseSplit(layout: SessionLayout, axis: 'row' | 'column', stacked: boolean): SessionLayout {
  if (layout.panes.length < 2) return { ...toggleSplit(layout), splitAxis: axis }
  if ((axis === 'column') === stacked) return { ...toggleSplit(layout), splitAxis: axis }
  return { ...layout, splitAxis: axis }
}

export function resizeSplit(layout: SessionLayout, ratio: number): SessionLayout {
  const splitRatio = Math.min(MAX_SPLIT_RATIO, Math.max(MIN_SPLIT_RATIO, ratio))
  return splitRatio === layout.splitRatio ? layout : { ...layout, splitRatio }
}

/**
 * `gap` counts tabs in the target pane as they stand before the move. The
 * Terminal tab stays first and stays put, so nothing lands ahead of it.
 */
export function moveTab(layout: SessionLayout, tabId: string, target: number, gap: number): SessionLayout {
  const from = paneWithTab(layout, tabId)
  const destination = layout.panes[target]
  if (tabId === TERMINAL_TAB_ID || from < 0 || !destination) return layout
  const tab = layout.panes[from].tabs.find((candidate) => candidate.id === tabId)!
  const position = from === target ? destination.tabs.indexOf(tab) : -1
  const adjusted = position >= 0 && position < gap ? gap - 1 : gap
  const cleared = removeTab(layout, from, tabId)
  const remaining = cleared.panes[target].tabs
  const at = Math.min(remaining.length, Math.max(target === 0 ? 1 : 0, adjusted))
  const placed = withPane(cleared, target, { tabs: [...remaining.slice(0, at), tab, ...remaining.slice(at)] })
  return dropEmptySecondPane(selectTab(placed, target, tabId))
}

/** The tree or a chord asked for a window: focus it where it shows, or show it in the Terminal tab. */
export function revealWindow(layout: SessionLayout, windowId: string): SessionLayout {
  const id = windowTabId(windowId)
  const existing = paneWithTab(layout, id)
  if (existing >= 0) return selectTab(layout, existing, id)
  return selectTab({ ...layout, terminalWindow: windowId }, 0, TERMINAL_TAB_ID)
}

/**
 * Brings a layout in line with the windows tmux has. The same layout comes back
 * when nothing changed, so a poll cannot cost a storage write. With no free
 * window left, the Terminal tab takes one back from the second pane.
 */
export function reconcileWindows(layout: SessionLayout, windowIds: readonly string[], preferred = ''): SessionLayout {
  let next = layout
  for (const [index, pane] of layout.panes.entries()) {
    for (const tab of pane.tabs) {
      if (tab.kind === 'window' && !windowIds.includes(tab.windowId)) next = removeTab(next, index, tab.id)
    }
  }
  if (!windowIds.includes(next.terminalWindow)) {
    const placed = next.panes.flatMap((pane) => pane.tabs.flatMap((tab) => (tab.kind === 'window' ? tab.windowId : [])))
    const candidates = preferred ? [preferred, ...windowIds] : [...windowIds]
    const terminalWindow = candidates.find((id) => !placed.includes(id)) ?? windowIds[0] ?? ''
    next = { ...next, terminalWindow }
    const holder = paneWithTab(next, windowTabId(terminalWindow))
    if (holder >= 0) next = removeTab(next, holder, windowTabId(terminalWindow))
  }
  return next === layout ? layout : dropEmptySecondPane(next)
}

function shownTab(pane: SessionPane): SessionTab | undefined {
  return pane.picking ? undefined : pane.tabs.find((tab) => tab.id === pane.activeTab)
}

function windowOf(layout: SessionLayout, tab: SessionTab | undefined): string {
  if (tab?.kind === 'terminal') return layout.terminalWindow
  return tab?.kind === 'window' ? tab.windowId : ''
}

/** The Docs, Diff and Tasks tabs a session has open, with where each one lives. */
export function openViews(layout: SessionLayout): { pane: number; tab: SessionTab }[] {
  return layout.panes.flatMap((pane, index) =>
    pane.tabs.filter((tab) => tab.kind !== 'terminal' && tab.kind !== 'window').map((tab) => ({ pane: index, tab })),
  )
}

/** The tab the focused pane shows, when nothing is asking there. */
export function focusedTab(layout: SessionLayout): string {
  const pane = layout.panes[layout.focusedPane]
  return pane ? (shownTab(pane)?.id ?? '') : ''
}

/** The window the keyboard belongs to, or '' with a view in the focused pane. */
export function focusedWindow(layout: SessionLayout): string {
  const pane = layout.panes[layout.focusedPane]
  return pane ? windowOf(layout, shownTab(pane)) : ''
}

/** Which pane shows each window on screen. */
export function windowsOnScreen(layout: SessionLayout): Map<string, number> {
  const shown = new Map<string, number>()
  layout.panes.forEach((pane, index) => {
    const windowId = windowOf(layout, shownTab(pane))
    if (windowId) shown.set(windowId, index)
  })
  return shown
}

/** Windows with no tab of their own and not in the Terminal tab: what a new pane can offer. */
export function freeWindows(layout: SessionLayout, windowIds: readonly string[]): string[] {
  return windowIds.filter(
    (id) =>
      id !== layout.terminalWindow && !layout.panes.some((pane) => pane.tabs.some((tab) => tab.id === windowTabId(id))),
  )
}
