import { onBeforeUnmount, watch } from 'vue'
import { appletTab } from '../../lib/appletTabs'
import { openTab, type PanelLayout } from '../../lib/panelLayout'
import { useApplets, type AppletTabHost } from '../../stores/useApplets'

/** Says this session area can show an applet as a tab while the user is in it. */
export function useAppletTabHost(options: {
  active: () => boolean
  apply: (rule: (layout: PanelLayout) => PanelLayout) => void
  sessionSlug: () => string
}): void {
  const applets = useApplets()
  const host: AppletTabHost = {
    open: (id) => options.apply((current) => openTab(current, appletTab(id))),
    sessionSlug: options.sessionSlug,
  }
  watch(options.active, (active) => applets.setTabHost(host, active), { immediate: true })
  onBeforeUnmount(() => applets.setTabHost(host, false))
}
