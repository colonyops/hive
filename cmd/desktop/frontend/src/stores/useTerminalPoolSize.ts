import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalPoolSize as PersistTerminalPoolSize,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

// How many sessions the terminal view keeps attached at once for instant
// switching (ADR terminal-attach-pool). Shared by SettingsView's control and
// TerminalMode's pool; settings.yaml is the only store.
export const terminalPoolSizes = [1, 2, 3, 4, 5, 6] as const
const DEFAULT_POOL_SIZE = 3

// The ceiling tracks the WebGL context budget: each pooled window's atlas
// renderer holds a context, and browsers cap those around sixteen.
export function resolveTerminalPoolSize(value: number): number {
  return Number.isInteger(value) && value >= 1 && value <= 6 ? value : DEFAULT_POOL_SIZE
}

export const useTerminalPoolSize = defineStore('terminalPoolSize', () => {
  const poolSize = usePersistedSetting({
    initial: DEFAULT_POOL_SIZE,
    read: async () => resolveTerminalPoolSize((await GetAppearanceSettings()).terminalPoolSize),
    write: (next) => PersistTerminalPoolSize(next),
    label: 'the terminal pool size',
  })

  function setPoolSize(next: number): void {
    poolSize.set(resolveTerminalPoolSize(next))
  }

  return { poolSize: poolSize.value, setPoolSize }
})
