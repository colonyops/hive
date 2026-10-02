import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalShowWindows as PersistTerminalShowWindows,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

// Whether the terminal sidebar lists every active session's windows, not just
// the attached session's. Shared by SettingsView's toggle and the terminal
// sidebar; settings.yaml is the only store and ships with the listing on.
export const useTerminalShowWindows = defineStore('terminalShowWindows', () => {
  const showWindows = usePersistedSetting({
    initial: true,
    read: async () => (await GetAppearanceSettings()).terminalShowWindows,
    write: PersistTerminalShowWindows,
    label: 'the terminal window listing setting',
  })

  function setShowWindows(next: boolean): void {
    showWindows.set(next)
  }

  // `ready` says the value is the stored one rather than the optimistic
  // default. The tree waits on it: painting subtrees the setting then turns
  // off is a wave of rows that arrive only to leave again.
  return { showWindows: showWindows.value, ready: showWindows.hydrated, setShowWindows }
})
