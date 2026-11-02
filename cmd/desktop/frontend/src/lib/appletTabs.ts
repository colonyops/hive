import { orderedTabs, type PanelLayout, type PanelTab } from './panelLayout'

const APPLET_TAB_KIND = 'applet'
const APPLET_TAB_PREFIX = `${APPLET_TAB_KIND}:`

export type AppletTabId = `applet:${string}`

/** The name an agent and the layout both use for an applet's tab. A session has one per applet. */
export function appletTabId(id: string): AppletTabId {
  return `applet:${id}`
}

export function isAppletTabId(id: string | undefined): id is AppletTabId {
  return !!id && id.startsWith(APPLET_TAB_PREFIX) && id.length > APPLET_TAB_PREFIX.length
}

export function appletTab(id: string): PanelTab {
  return { id: appletTabId(id), kind: APPLET_TAB_KIND, params: { applet: id } }
}

export function appletIdOf(tab: PanelTab | undefined): string | undefined {
  if (tab?.kind !== APPLET_TAB_KIND) return undefined
  const id = tab.params?.applet
  return typeof id === 'string' ? id : undefined
}

/**
 * Every applet tab of a layout, in an order no drag can change. Moving an iframe
 * in the DOM reloads its page, so the floating layer must never reorder these.
 */
export function appletTabsOf(layout: PanelLayout): { panel: string; tab: PanelTab; applet: string }[] {
  return orderedTabs(layout)
    .flatMap(({ panel, tab }) => {
      const applet = appletIdOf(tab)
      return applet ? [{ panel, tab, applet }] : []
    })
    .sort((a, b) => a.tab.id.localeCompare(b.tab.id))
}
