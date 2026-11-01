import {
  focusedTab,
  orderedTabs,
  panelWithTab,
  panelsOf,
  placeTab,
  selectTab,
  shownTab,
  shownTabs,
  syncTabs,
  tabById,
  type PanelLayout,
  type PanelTab,
} from './panelLayout'

/**
 * The tab kind that shows a tmux window. A window has exactly one tab, so the
 * tab's id is the window's, and a layout's window tabs mirror the session's
 * windows: `reconcileWindows` gives every window a tab and takes the tab away
 * with its window. Nothing else in a layout is tmux's.
 */
export const WINDOW_TAB_KIND = 'window'

export function windowTabId(windowId: string): string {
  return `window:${windowId}`
}

export function windowTab(windowId: string): PanelTab {
  return { id: windowTabId(windowId), kind: WINDOW_TAB_KIND, params: { windowId } }
}

/** The window a tab shows, or nothing for a view. */
export function windowIdOf(tab: PanelTab | undefined): string | undefined {
  if (tab?.kind !== WINDOW_TAB_KIND) return undefined
  const windowId = tab.params?.windowId
  return typeof windowId === 'string' ? windowId : undefined
}

/** The same, for a tab named by id alone, as a drag payload names it. */
export function windowIdOfTab(layout: PanelLayout, tabId: string): string | undefined {
  return windowIdOf(tabById(layout, tabId))
}

export function reconcileWindows(
  layout: PanelLayout,
  windowIds: readonly string[],
  options: { active?: string; target?: string } = {},
): PanelLayout {
  return syncTabs(layout, WINDOW_TAB_KIND, windowIds, windowTab, {
    active: options.active ? windowTabId(options.active) : undefined,
    target: options.target,
  })
}

/** A window has one tab, so placing one moves that tab here. */
export function placeWindow(layout: PanelLayout, windowId: string, target: string): PanelLayout {
  return placeTab(layout, windowTab(windowId), target)
}

/**
 * The tree or a chord asked for a window: focus its tab, wherever it lives. A
 * session still attaching has no tab for it yet. Making one here would put that
 * window ahead of the others, so the active window brings it forward once it lands.
 */
export function revealWindow(layout: PanelLayout, windowId: string): PanelLayout {
  const id = windowTabId(windowId)
  const panel = panelWithTab(layout, id)
  return panel ? selectTab(layout, panel.id, id) : layout
}

/** Windows in the order the tabs show them. */
export function windowOrder(layout: PanelLayout): string[] {
  return orderedTabs(layout).flatMap(({ tab }) => {
    const windowId = windowIdOf(tab)
    return windowId ? [windowId] : []
  })
}

/**
 * Windows in the order the tabs and the tree show them. tmux's own can differ,
 * as when a closed panel's tabs join the end of its neighbour's, and one move
 * cannot always repair it. A window with no tab yet goes last.
 */
export function inTabOrder<T extends { windowId: string }>(windows: readonly T[], layout: PanelLayout): T[] {
  const order = windowOrder(layout)
  const position = (window: T): number => {
    const at = order.indexOf(window.windowId)
    return at < 0 ? order.length : at
  }
  return [...windows].sort((a, b) => position(a) - position(b))
}

/** The window the keyboard belongs to, or '' with a view in the focused panel. */
export function focusedWindow(layout: PanelLayout): string {
  return windowIdOf(tabById(layout, focusedTab(layout))) ?? ''
}

/** Which panel shows each window on screen. */
export function windowsOnScreen(layout: PanelLayout): Map<string, string> {
  const shown = new Map<string, string>()
  for (const [panelId, tab] of shownTabs(layout)) {
    const windowId = windowIdOf(tab)
    if (windowId) shown.set(windowId, panelId)
  }
  return shown
}

/** Windows behind another tab: what a new panel can bring forward. */
export function offscreenWindows(layout: PanelLayout, windowIds: readonly string[]): string[] {
  const shown = windowsOnScreen(layout)
  return windowIds.filter((id) => !shown.has(id))
}

/** The panel a session's terminal states show in: one showing a window, or nothing at all. */
export function terminalPanel(layout: PanelLayout): string {
  const panel = panelsOf(layout).find((candidate) => {
    if (candidate.picking) return false
    const tab = shownTab(candidate)
    return !tab || tab.kind === WINDOW_TAB_KIND
  })
  return panel?.id ?? ''
}
