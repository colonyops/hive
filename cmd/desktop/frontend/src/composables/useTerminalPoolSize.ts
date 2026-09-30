import { ref, type Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalPoolSize as PersistTerminalPoolSize,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

// How many sessions the terminal view keeps attached at once for instant
// switching (ADR terminal-attach-pool). A module singleton like useTerminalShowWindows,
// shared by SettingsView's control and TerminalMode's pool; settings.yaml is
// the only store.
export const terminalPoolSizes = [1, 2, 3, 4, 5, 6] as const
const DEFAULT_POOL_SIZE = 3

// The ceiling tracks the WebGL context budget: each pooled window's atlas
// renderer holds a context, and browsers cap those around sixteen.
export function resolveTerminalPoolSize(value: number): number {
  return Number.isInteger(value) && value >= 1 && value <= 6 ? value : DEFAULT_POOL_SIZE
}

const poolSize: Ref<number> = ref(DEFAULT_POOL_SIZE)

let hydrated = false
// Same staleness guard as useTerminalFont: a change made while the hydrating
// read is in flight must not be overwritten by its result.
let version = 0
let persistChain: Promise<void> = Promise.resolve()

async function hydrate(): Promise<void> {
  const startedAt = version
  try {
    const settings = await GetAppearanceSettings()
    if (version !== startedAt) return
    poolSize.value = resolveTerminalPoolSize(settings.terminalPoolSize)
  } catch (error) {
    console.warn('Unable to load the terminal pool size from settings.yaml', error)
  }
}

export function setTerminalPoolSize(next: number): void {
  const size = resolveTerminalPoolSize(next)
  version++
  poolSize.value = size
  // Chained so two quick changes cannot land out of order.
  persistChain = persistChain
    .then(async () => {
      await PersistTerminalPoolSize(size)
    })
    .catch((error: unknown) => {
      console.warn('Unable to persist the terminal pool size to settings.yaml', error)
    })
}

export function useTerminalPoolSize(): { poolSize: Ref<number> } {
  if (!hydrated) {
    hydrated = true
    void hydrate()
  }
  return { poolSize }
}
