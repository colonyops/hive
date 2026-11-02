import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalMonochromeAgentIcons as PersistMonochrome,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

export const useTerminalAgentIcons = defineStore('terminalAgentIcons', () => {
  const setting = usePersistedSetting({
    initial: false,
    read: async () => (await GetAppearanceSettings()).terminalMonochromeAgentIcons,
    write: (next) => PersistMonochrome(next),
    label: 'the agent icon setting',
  })

  function setMonochrome(next: boolean): void {
    setting.set(next)
  }

  return { monochrome: setting.value, setMonochrome }
})
