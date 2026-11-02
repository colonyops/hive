import {
  addTabBeside,
  addTabQuietly,
  closePanel,
  focusTab,
  moveTabBehind,
  moveTabToSplit,
  panelById,
  panelWithTab,
  resizeSplitFor,
  turnSplitFor,
  type Panel,
  type PanelLayout,
  type PanelTab,
  type SplitAxis,
} from '../../lib/panelLayout'
import { windowTabId } from '../../lib/windowTabs'
import { viewTab } from '../terminal/panelViews'
import type { SessionTabPlacement, TabRef } from './useSessionLayout'

/**
 * The layout tab id a ref names. A terminal with no window is the host's
 * own window, which only Chats has, so `ownWindowId` is how it says so.
 */
export function tabIdOf(ref: TabRef, ownWindowId?: string): string | undefined {
  if (ref.view !== 'terminal') return viewTab(ref.view).id
  const windowId = ref.window || ownWindowId
  return windowId ? windowTabId(windowId) : undefined
}

/** The panel holding the tab a ref names, or nothing when that tab is not open. */
export function panelOfRef(layout: PanelLayout, ref: TabRef, ownWindowId?: string): Panel | undefined {
  const id = tabIdOf(ref, ownWindowId)
  return id ? panelWithTab(layout, id) : undefined
}

/** The panel an open is placed against: the anchor's, or the focused one for no anchor or one that is not open. */
export function anchorPanelOf(layout: PanelLayout, anchor: TabRef | null, ownWindowId?: string): string {
  return (anchor && panelOfRef(layout, anchor, ownWindowId)?.id) || layout.focusedPanel
}

export function axisOf(placement: SessionTabPlacement): SplitAxis {
  return placement === 'below' ? 'column' : 'row'
}

/**
 * A view an agent opened. Behind with no anchor joins the focused panel behind
 * the tab in front, so the user's current tab stays in front. Behind an anchor
 * that is open goes right after it, in its panel.
 */
export function openViewRule(
  layout: PanelLayout,
  tab: PanelTab,
  placement: SessionTabPlacement,
  anchor: TabRef | null,
  ownWindowId?: string,
): PanelLayout {
  if (placement !== 'behind') {
    return addTabBeside(layout, tab, axisOf(placement), anchorPanelOf(layout, anchor, ownWindowId))
  }
  if (panelWithTab(layout, tab.id)) return layout
  const placed = addTabQuietly(layout, tab)
  const anchorId = anchor && panelOfRef(layout, anchor, ownWindowId) ? tabIdOf(anchor, ownWindowId) : undefined
  return anchorId ? moveTabBehind(placed, tab.id, anchorId) : placed
}

/** A tab or an anchor that is not open leaves the layout as it is. */
export function moveRule(
  layout: PanelLayout,
  tab: TabRef,
  placement: SessionTabPlacement,
  anchor: TabRef | null,
  ownWindowId?: string,
): PanelLayout {
  const tabId = tabIdOf(tab, ownWindowId)
  if (!tabId || !panelWithTab(layout, tabId)) return layout
  const anchorPanel = anchor ? panelOfRef(layout, anchor, ownWindowId) : undefined
  if (anchor && !anchorPanel) return layout
  if (placement === 'behind') return moveTabBehind(layout, tabId, anchor ? tabIdOf(anchor, ownWindowId) : undefined)
  return moveTabToSplit(layout, tabId, axisOf(placement), anchorPanel?.id ?? layout.focusedPanel)
}

export function focusRule(layout: PanelLayout, tab: TabRef, ownWindowId?: string): PanelLayout {
  const tabId = tabIdOf(tab, ownWindowId)
  return tabId ? focusTab(layout, tabId) : layout
}

/** Closing a panel merges it into its neighbour; the agent's verb does not move the keyboard while its panel survives. */
export function closePanelRule(layout: PanelLayout, tab: TabRef, ownWindowId?: string): PanelLayout {
  const panel = panelOfRef(layout, tab, ownWindowId)
  if (!panel) return layout
  const closed = closePanel(layout, panel.id)
  if (closed === layout || closed.focusedPanel === layout.focusedPanel) return closed
  return panelById(closed, layout.focusedPanel) ? { ...closed, focusedPanel: layout.focusedPanel } : closed
}

export function resizeRule(layout: PanelLayout, tab: TabRef, share: number, ownWindowId?: string): PanelLayout {
  const panel = panelOfRef(layout, tab, ownWindowId)
  return panel ? resizeSplitFor(layout, panel.id, share) : layout
}

export function turnRule(layout: PanelLayout, tab: TabRef, ownWindowId?: string): PanelLayout {
  const panel = panelOfRef(layout, tab, ownWindowId)
  return panel ? turnSplitFor(layout, panel.id) : layout
}
