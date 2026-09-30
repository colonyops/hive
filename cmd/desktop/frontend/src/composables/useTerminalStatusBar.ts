import { ref, type Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalShowStatusBar as PersistTerminalShowStatusBar,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

// A module singleton like useTerminalShowWindows's, shared by SettingsView's
// toggle and the terminal pane; settings.yaml is the only store and ships
// with the bar on.
const showStatusBar: Ref<boolean> = ref(true)

let hydrated = false
// A toggle made while the hydrating read is in flight must not be overwritten
// by its result.
let version = 0
let persistChain: Promise<void> = Promise.resolve()

async function hydrate(): Promise<void> {
  const startedAt = version
  try {
    const settings = await GetAppearanceSettings()
    if (version !== startedAt) return
    showStatusBar.value = settings.terminalShowStatusBar
  } catch (error) {
    console.warn('Unable to load the session status bar setting from settings.yaml', error)
  }
}

export function setTerminalShowStatusBar(next: boolean): void {
  version++
  showStatusBar.value = next
  // Chained so two quick toggles cannot land out of order.
  persistChain = persistChain
    .then(async () => {
      await PersistTerminalShowStatusBar(next)
    })
    .catch((error: unknown) => {
      console.warn('Unable to persist the session status bar setting to settings.yaml', error)
    })
}

export function useTerminalStatusBar(): { showStatusBar: Ref<boolean> } {
  if (!hydrated) {
    hydrated = true
    void hydrate()
  }
  return { showStatusBar }
}
