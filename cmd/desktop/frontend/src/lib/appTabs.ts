import type { RouteLocationRaw, RouteParamsGeneric, RouteRecordNameGeneric } from 'vue-router'
import { appletTabId, isAppletTabId, type AppletTabId } from './appletTabs'

/** The three areas every window has. They hold the left of the strip and never close. */
export type PinnedTabId = 'inbox' | 'code' | 'chats'
/** A built-in page the user can add to the strip. */
export type ViewTabId = 'drive' | 'tasks' | 'settings'
/** A tab the user added, closed, and can drag: a built-in page or an applet. */
export type GlobalTabId = ViewTabId | AppletTabId
export type AppTabId = PinnedTabId | GlobalTabId

export const PINNED_TABS: readonly PinnedTabId[] = ['inbox', 'code', 'chats']
export const VIEW_TABS: readonly ViewTabId[] = ['drive', 'tasks', 'settings']

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
    case 'settings':
      return { name: 'application-settings' }
    default:
      return { name: 'applet', params: { id: id.slice('applet:'.length) } }
  }
}
