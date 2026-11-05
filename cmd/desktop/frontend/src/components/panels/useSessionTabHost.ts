import { onBeforeUnmount, watch } from 'vue'
import { openTab, type PanelLayout } from '../../lib/panelLayout'
import { useSessionTabHost as useSessionTabHostStore, type SessionTabHost } from '../../stores/useSessionTabHost'
import { focusShownTab } from './panelTabRow'
import { viewTab, type SessionViewKind } from '../terminal/panelViews'

/** Says this session area can open its session's views from the keymap and the palette while the user is in it. */
export function useSessionTabHost(options: {
  active: () => boolean
  apply: (rule: (layout: PanelLayout) => PanelLayout) => void
  sessionName: () => string
  views: () => readonly SessionViewKind[]
}): void {
  const store = useSessionTabHostStore()
  const host: SessionTabHost = {
    sessionName: options.sessionName,
    views: options.views,
    open: (kind) => {
      const tab = viewTab(kind)
      options.apply((current) => openTab(current, tab))
      focusShownTab(tab.id)
    },
  }
  watch(options.active, (active) => store.setHost(host, active), { immediate: true })
  onBeforeUnmount(() => store.setHost(host, false))
}
