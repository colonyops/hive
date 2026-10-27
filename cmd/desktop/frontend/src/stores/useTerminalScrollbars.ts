import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalShowScrollbars as PersistTerminalShowScrollbars,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

// Matches xterm's default vertical scrollbar width.
export const terminalScrollbarWidth = 14

export const useTerminalScrollbars = defineStore('terminalScrollbars', () => {
  const setting = usePersistedSetting({
    initial: false,
    read: async () => (await GetAppearanceSettings()).terminalShowScrollbars,
    write: (next) => PersistTerminalShowScrollbars(next),
    label: 'the terminal scrollbar setting',
  })

  function setShowScrollbars(next: boolean): void {
    setting.set(next)
  }

  return { showScrollbars: setting.value, setShowScrollbars }
})

export function scrollbarWidthFor(tab: { panes: readonly unknown[]; zoomed: boolean }, enabled: boolean): number {
  return enabled && (tab.zoomed || tab.panes.length === 1) ? terminalScrollbarWidth : 0
}
