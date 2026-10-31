export type SessionViewKind = 'docs' | 'diff' | 'tasks'

/** `auto` is side by side until the session gets too narrow, then stacked. A pick holds at any width. */
export type SplitAxis = 'auto' | 'row' | 'column'

export const MIN_SPLIT_RATIO = 0.2
export const MAX_SPLIT_RATIO = 0.8

export type SessionTab = { id: string; kind: 'window'; windowId: string } | { id: string; kind: SessionViewKind }

export interface SessionPane {
  tabs: SessionTab[]
  activeTab: string | null
  /** An empty pane asking what goes in it. */
  picking: boolean
}

/**
 * What a session shows in Code. Every tmux window has exactly one tab, and the
 * tree lists these tabs in this order, so the two always mirror each other. Ids
 * derive from the content, so opening a view twice finds the tab it already has.
 */
export interface SessionLayout {
  panes: SessionPane[]
  focusedPane: number
  docPath: string | null
  splitAxis: SplitAxis
  /** The first pane's share of the session. */
  splitRatio: number
}

export function newSessionLayout(): SessionLayout {
  return {
    panes: [{ tabs: [], activeTab: null, picking: false }],
    focusedPane: 0,
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
  return (
    panes.every((pane) => Array.isArray(pane?.tabs)) &&
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

export function windowTabId(windowId: string): string {
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
  if (from >= 0) return moveTab(layout, kind, target, layout.panes[target].tabs.length)
  return appendTab(layout, target, { id: kind, kind })
}

/** A window has one tab, so placing one moves that tab here. */
export function placeWindow(layout: SessionLayout, windowId: string, target: number): SessionLayout {
  const id = windowTabId(windowId)
  const from = paneWithTab(layout, id)
  if (from === target) return selectTab(layout, target, id)
  if (from >= 0) return moveTab(layout, id, target, layout.panes[target].tabs.length)
  return appendTab(layout, target, { id, kind: 'window', windowId })
}

function removeTab(layout: SessionLayout, index: number, tabId: string): SessionLayout {
  const pane = layout.panes[index]
  const at = pane.tabs.findIndex((tab) => tab.id === tabId)
  const tabs = pane.tabs.filter((tab) => tab.id !== tabId)
  const activeTab = pane.activeTab === tabId ? ((tabs[at] ?? tabs[at - 1])?.id ?? null) : pane.activeTab
  return withPane(layout, index, { tabs, activeTab })
}

/** A pane with nothing in it closes, whichever side it is on. The last pane stays. */
function dropEmptyPane(layout: SessionLayout): SessionLayout {
  if (layout.panes.length < 2) return layout
  const kept = layout.panes.filter((pane) => pane.tabs.length || pane.picking)
  if (kept.length === layout.panes.length) return layout
  return { ...layout, panes: kept.length ? kept : [layout.panes[0]], focusedPane: 0 }
}

/** Removes a tab from the layout. A window's tab goes when its tmux window goes: see reconcileWindows. */
export function closeTab(layout: SessionLayout, index: number, tabId: string): SessionLayout {
  if (!layout.panes[index]?.tabs.some((tab) => tab.id === tabId)) return layout
  return dropEmptyPane(removeTab(layout, index, tabId))
}

/** Everything in the second pane survives the merge: a window keeps its tab. */
export function toggleSplit(layout: SessionLayout): SessionLayout {
  const [first, second] = layout.panes
  if (!second) {
    return {
      ...layout,
      panes: [first, { tabs: [], activeTab: null, picking: true }],
      focusedPane: 1,
    }
  }
  return {
    ...layout,
    panes: [{ ...first, tabs: [...first.tabs, ...second.tabs], activeTab: first.activeTab ?? second.activeTab }],
    focusedPane: 0,
  }
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

/** `gap` counts tabs in the target pane as they stand before the move. */
export function moveTab(layout: SessionLayout, tabId: string, target: number, gap: number): SessionLayout {
  const from = paneWithTab(layout, tabId)
  const destination = layout.panes[target]
  if (from < 0 || !destination) return layout
  const tab = layout.panes[from].tabs.find((candidate) => candidate.id === tabId)!
  const position = from === target ? destination.tabs.indexOf(tab) : -1
  const adjusted = position >= 0 && position < gap ? gap - 1 : gap
  const cleared = removeTab(layout, from, tabId)
  const remaining = cleared.panes[target].tabs
  const at = Math.min(remaining.length, Math.max(0, adjusted))
  const placed = withPane(cleared, target, { tabs: [...remaining.slice(0, at), tab, ...remaining.slice(at)] })
  return dropEmptyPane(selectTab(placed, target, tabId))
}

/** The tree names a neighbour rather than a pane and a gap. */
export function moveTabBeside(
  layout: SessionLayout,
  tabId: string,
  besideId: string,
  edge: 'before' | 'after',
): SessionLayout {
  const pane = paneWithTab(layout, besideId)
  if (pane < 0) return layout
  const at = layout.panes[pane].tabs.findIndex((tab) => tab.id === besideId)
  return moveTab(layout, tabId, pane, edge === 'after' ? at + 1 : at)
}

/**
 * The tree or a chord asked for a window: focus its tab, wherever it lives. A
 * session still attaching has no tab for it yet. Making one here would put that
 * window ahead of the others, so the active window brings it forward once it lands.
 */
export function revealWindow(layout: SessionLayout, windowId: string): SessionLayout {
  const id = windowTabId(windowId)
  const existing = paneWithTab(layout, id)
  return existing >= 0 ? selectTab(layout, existing, id) : layout
}

/**
 * Gives every window tmux has a tab, and takes the tab away from one it no
 * longer has. The same layout comes back when nothing changed, so a poll cannot
 * cost a storage write. A pane with nothing active takes `active` when it holds
 * that window, and its first tab otherwise.
 */
export function reconcileWindows(
  layout: SessionLayout,
  windowIds: readonly string[],
  options: { active?: string; target?: number } = {},
): SessionLayout {
  let next = layout
  for (const [index, pane] of layout.panes.entries()) {
    for (const tab of pane.tabs) {
      if (tab.kind === 'window' && !windowIds.includes(tab.windowId)) next = removeTab(next, index, tab.id)
    }
  }
  const target = next.panes[options.target ?? next.focusedPane] ? (options.target ?? next.focusedPane) : 0
  const missing = windowIds.filter((id) => paneWithTab(next, windowTabId(id)) < 0)
  if (missing.length) {
    const added = missing.map((windowId) => ({ id: windowTabId(windowId), kind: 'window' as const, windowId }))
    next = withPane(next, target, { tabs: [...next.panes[target].tabs, ...added], picking: false })
  }
  for (const [index, pane] of next.panes.entries()) {
    if (pane.activeTab || !pane.tabs.length) continue
    const wanted = options.active ? windowTabId(options.active) : ''
    next = withPane(next, index, {
      activeTab: pane.tabs.some((tab) => tab.id === wanted) ? wanted : pane.tabs[0].id,
    })
  }
  return next === layout ? layout : dropEmptyPane(next)
}

function shownTab(pane: SessionPane): SessionTab | undefined {
  return pane.picking ? undefined : pane.tabs.find((tab) => tab.id === pane.activeTab)
}

/** Every tab in the order the tree lists it, with where each one lives. */
export function orderedTabs(layout: SessionLayout): { pane: number; tab: SessionTab }[] {
  return layout.panes.flatMap((pane, index) => pane.tabs.map((tab) => ({ pane: index, tab })))
}

export function windowOrder(layout: SessionLayout): string[] {
  return orderedTabs(layout).flatMap(({ tab }) => (tab.kind === 'window' ? [tab.windowId] : []))
}

/** The tab the focused pane shows, when nothing is asking there. */
export function focusedTab(layout: SessionLayout): string {
  const pane = layout.panes[layout.focusedPane]
  return pane ? (shownTab(pane)?.id ?? '') : ''
}

/** The window the keyboard belongs to, or '' with a view in the focused pane. */
export function focusedWindow(layout: SessionLayout): string {
  const pane = layout.panes[layout.focusedPane]
  const tab = pane && shownTab(pane)
  return tab?.kind === 'window' ? tab.windowId : ''
}

/** Which pane shows each window on screen. */
export function windowsOnScreen(layout: SessionLayout): Map<string, number> {
  const shown = new Map<string, number>()
  layout.panes.forEach((pane, index) => {
    const tab = shownTab(pane)
    if (tab?.kind === 'window') shown.set(tab.windowId, index)
  })
  return shown
}

/** Windows behind another tab: what a new pane can bring forward. */
export function offscreenWindows(layout: SessionLayout, windowIds: readonly string[]): string[] {
  const shown = windowsOnScreen(layout)
  return windowIds.filter((id) => !shown.has(id))
}

/** The pane a session's terminal states show in: one holding a window, or nothing at all. */
export function terminalPane(layout: SessionLayout): number {
  return layout.panes.findIndex((pane) => {
    if (pane.picking) return false
    const tab = shownTab(pane)
    return !tab || tab.kind === 'window'
  })
}
