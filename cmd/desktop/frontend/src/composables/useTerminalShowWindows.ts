import { ref, type Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalShowWindows as PersistTerminalShowWindows,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

// Whether the terminal sidebar lists every active session's windows, not just
// the attached session's. A module singleton like useTerminalFont's size,
// shared by SettingsView's toggle and the terminal sidebar; settings.yaml is
// the only store and ships with the listing on.
const showWindows: Ref<boolean> = ref(true)

// Whether the value above is the stored one rather than the optimistic default.
// The tree waits on it: painting subtrees the setting then turns off is a wave
// of rows that arrive only to leave again.
const ready: Ref<boolean> = ref(false)

let hydrated = false
// Same staleness guard as useTerminalFont: a toggle made while the hydrating
// read is in flight must not be overwritten by its result.
let version = 0
let persistChain: Promise<void> = Promise.resolve()

async function hydrate(): Promise<void> {
  const startedAt = version
  try {
    const settings = await GetAppearanceSettings()
    if (version !== startedAt) return
    showWindows.value = settings.terminalShowWindows
  } catch (error) {
    console.warn('Unable to load the terminal window listing setting from settings.yaml', error)
  } finally {
    ready.value = true
  }
}

export function setTerminalShowWindows(next: boolean): void {
  version++
  showWindows.value = next
  // Chained so two quick toggles cannot land out of order.
  persistChain = persistChain
    .then(async () => {
      await PersistTerminalShowWindows(next)
    })
    .catch((error: unknown) => {
      console.warn('Unable to persist the terminal window listing setting to settings.yaml', error)
    })
}

export function useTerminalShowWindows(): { showWindows: Ref<boolean>; ready: Ref<boolean> } {
  if (!hydrated) {
    hydrated = true
    void hydrate()
  }
  return { showWindows, ready }
}
