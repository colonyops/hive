import { computed } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalSessionAge as PersistTerminalSessionAge,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

export const defaultTerminalSessionAgeThresholdDays = 5
export const minTerminalSessionAgeThresholdDays = 1
export const maxTerminalSessionAgeThresholdDays = 365

const dayMilliseconds = 24 * 60 * 60 * 1000

interface TerminalSessionAgeSetting {
  enabled: boolean
  thresholdDays: number
}

export function resolveTerminalSessionAgeThresholdDays(value: number): number {
  return Number.isInteger(value) &&
    value >= minTerminalSessionAgeThresholdDays &&
    value <= maxTerminalSessionAgeThresholdDays
    ? value
    : defaultTerminalSessionAgeThresholdDays
}

export function sessionAgeDays(createdAt: string | undefined, thresholdDays: number, now = Date.now()): number | null {
  if (!createdAt) return null
  const created = Date.parse(createdAt)
  if (!Number.isFinite(created) || created > now) return null
  const days = Math.floor((now - created) / dayMilliseconds)
  return days >= thresholdDays ? days : null
}

export const useTerminalSessionAge = defineStore('terminalSessionAge', () => {
  const setting = usePersistedSetting<TerminalSessionAgeSetting>({
    initial: { enabled: false, thresholdDays: defaultTerminalSessionAgeThresholdDays },
    read: async () => {
      const current = await GetAppearanceSettings()
      return {
        enabled: current.terminalShowSessionAge,
        thresholdDays: resolveTerminalSessionAgeThresholdDays(current.terminalSessionAgeThresholdDays),
      }
    },
    write: (next) => PersistTerminalSessionAge(next.enabled, next.thresholdDays),
    label: 'the terminal session age setting',
  })

  const enabled = computed(() => setting.value.value.enabled)
  const thresholdDays = computed(() => setting.value.value.thresholdDays)

  function setEnabled(next: boolean): void {
    setting.set({ enabled: next, thresholdDays: thresholdDays.value })
  }

  function setThresholdDays(next: number): void {
    setting.set({ enabled: enabled.value, thresholdDays: resolveTerminalSessionAgeThresholdDays(next) })
  }

  return { enabled, thresholdDays, setEnabled, setThresholdDays }
})
