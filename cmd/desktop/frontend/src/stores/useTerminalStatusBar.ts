import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalShowStatusBar as PersistTerminalShowStatusBar,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

// Shared by SettingsView's toggle and the terminal pane; settings.yaml is the
// only store and ships with the bar on.
export const useTerminalStatusBar = defineStore('terminalStatusBar', () => {
  const showStatusBar = usePersistedSetting({
    initial: true,
    read: async () => (await GetAppearanceSettings()).terminalShowStatusBar,
    write: (next) => PersistTerminalShowStatusBar(next),
    label: 'the session status bar setting',
  })

  function setShowStatusBar(next: boolean): void {
    showStatusBar.set(next)
  }

  return { showStatusBar: showStatusBar.value, setShowStatusBar }
})
