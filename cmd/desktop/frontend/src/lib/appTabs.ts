import type { RouteLocationRaw, RouteParamsGeneric, RouteRecordNameGeneric } from 'vue-router'
import { appletTabId, isAppletTabId, type AppletTabId } from './appletTabs'

/** The three areas every window has. They hold the left of the strip and never close. */
export type PinnedTabId = 'inbox' | 'code' | 'chats'
/** A built-in page the user can add to the strip. */
export type ViewTabId = 'drive' | 'tasks' | 'runs' | 'settings'
/** A tab the user added, closed, and can drag: a built-in page or an applet. */
export type GlobalTabId = ViewTabId | AppletTabId
export type AppTabId = PinnedTabId | GlobalTabId

export const PINNED_TABS: readonly PinnedTabId[] = ['inbox', 'code', 'chats']
export const VIEW_TABS: readonly ViewTabId[] = ['drive', 'tasks', 'runs', 'settings']

export function isViewTabId(value: unknown): value is ViewTabId {
  return typeof value === 'string' && (VIEW_TABS as readonly string[]).includes(value)
}

export function isGlobalTabId(value: unknown): value is GlobalTabId {
  return isViewTabId(value) || (typeof value === 'string' && isAppletTabId(value))
}

export function isPinnedTabId(value: unknown): value is PinnedTabId {
  return typeof value === 'string' && (PINNED_TABS as readonly string[]).includes(value)
}

/**
 * The tab a route belongs to. Every route has one, so the strip always has a
 * tab selected: the hub's pages (feed, flows, profile settings, dev) are Inbox.
 */
export function tabOfRoute(
  name: RouteRecordNameGeneric | null | undefined,
  params: RouteParamsGeneric,
): AppTabId | null {
  switch (name) {
    case undefined:
    case null:
      return null
    case 'terminal':
      return 'code'
    case 'agents':
      return 'chats'
    case 'drive':
      return 'drive'
    case 'tasks':
      return 'tasks'
    case 'action-runs':
      return 'runs'
    case 'application-settings':
      return 'settings'
    case 'applet':
      return typeof params.id === 'string' && params.id ? appletTabId(params.id) : null
    default:
      return 'inbox'
  }
}

/** Where a global tab opens when it has no page of its own to return to yet. */
export function globalTabLocation(id: GlobalTabId): RouteLocationRaw {
  switch (id) {
    case 'drive':
      return { name: 'drive' }
    case 'tasks':
      return { name: 'tasks' }
    case 'runs':
      return { name: 'action-runs' }
    case 'settings':
      return { name: 'application-settings' }
    default:
      return { name: 'applet', params: { id: id.slice('applet:'.length) } }
  }
}

/** How much of each tab the title-bar strip can afford to show. */
export interface TabStripDensity {
  globalLabels: boolean
  pinnedLabels: boolean
}

export interface TabStripSpace {
  /** The strip's own width, or 0 before it is measured. */
  width: number
  /** What Inbox, Code and Chats take with their labels. */
  pinnedLabeledWidth: number
  globalCount: number
  /** A global tab is in front, so it keeps its close button even as an icon. */
  globalSelected: boolean
}

// Matched to the classes in TitleBarTabs.vue.
const TAB_GAP = 4
const GLOBAL_LABELED_MIN = 84
const GLOBAL_ICON = 32
const CLOSE_BUTTON = 22
const ADD_BUTTON = 32
const SEPARATOR = 17

/**
 * Crunches the strip the way a browser does: global tabs shrink and truncate
 * down to a minimum, then drop to icons; the pinned tabs drop their labels
 * only once icon-only global tabs still do not fit. Past that the strip
 * scrolls. An unmeasured strip shows everything, so nothing flickers in on
 * mount and a test without layout sees the labels.
 */
export function tabStripDensity(space: TabStripSpace): TabStripDensity {
  const { width, pinnedLabeledWidth, globalCount, globalSelected } = space
  if (width <= 0) return { globalLabels: true, pinnedLabels: true }
  const room = width - pinnedLabeledWidth - ADD_BUTTON - (globalCount ? SEPARATOR : 0)
  const need = (perTab: number, extra = 0) => globalCount * perTab + Math.max(0, globalCount - 1) * TAB_GAP + extra
  if (room >= need(GLOBAL_LABELED_MIN)) return { globalLabels: true, pinnedLabels: true }
  if (room >= need(GLOBAL_ICON, globalSelected ? CLOSE_BUTTON : 0)) return { globalLabels: false, pinnedLabels: true }
  return { globalLabels: false, pinnedLabels: false }
}
