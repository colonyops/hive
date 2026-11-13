import { orderedTabs, type PanelLayout, type PanelTab } from './panelLayout'

// A native terminal tab shows one herdr-core terminal, drawn by libghostty
// over the webview. Its id carries the terminal's address, so a session can
// hold any number of them.
const NATIVE_TAB_KIND = 'hc'
const NATIVE_TAB_PREFIX = `${NATIVE_TAB_KIND}:`

export function nativeTerminalTab(address: string): PanelTab {
  return { id: `${NATIVE_TAB_PREFIX}${address}`, kind: NATIVE_TAB_KIND, params: { address } }
}

export function nativeAddressOf(tab: PanelTab | undefined): string | undefined {
  if (tab?.kind !== NATIVE_TAB_KIND) return undefined
  const address = tab.params?.address
  return typeof address === 'string' && address ? address : undefined
}

/** Every native terminal tab of a layout with the panel it sits in, in a stable order. */
export function nativeTerminalTabsOf(layout: PanelLayout): { panel: string; tab: PanelTab; address: string }[] {
  return orderedTabs(layout)
    .flatMap(({ panel, tab }) => {
      const address = nativeAddressOf(tab)
      return address ? [{ panel, tab, address }] : []
    })
    .sort((a, b) => a.tab.id.localeCompare(b.tab.id))
}
