import { useStorage } from '@vueuse/core'
import type { Ref } from 'vue'

export const themes = ['dark', 'light', 'midnight', 'gruvbox'] as const
export type Theme = (typeof themes)[number]

export const themeLabels: Record<Theme, string> = {
  dark: 'Dark',
  light: 'Light',
  midnight: 'Midnight',
  gruvbox: 'Gruvbox',
}

function isTheme(value: string | null): value is Theme {
  return themes.includes(value as Theme)
}

// The live theme, persisted in localStorage via VueUse. A module singleton
// (like useFlowsSession's shared session) so every caller — the command
// palette's theme:* commands, SettingsView's Appearance section — reads/drives
// the same value. useStorage loads the persisted theme at import; the dataset
// mirror is applied by initializeTheme() (called once in main.ts before mount)
// and thereafter by setTheme().
const currentTheme: Ref<Theme> = useStorage<Theme>('hive.theme', 'dark')

export function setTheme(nextTheme: Theme): void {
  document.documentElement.dataset.theme = nextTheme
  currentTheme.value = nextTheme // persisted by useStorage
}

// Called once before mount so the first paint uses the persisted theme, and to
// heal a garbage stored value back to the default.
export function initializeTheme(): void {
  setTheme(isTheme(currentTheme.value) ? currentTheme.value : 'dark')
}

/** The live theme, kept in sync by every setTheme()/initializeTheme() call. */
export function useTheme(): { theme: Ref<Theme> } {
  return { theme: currentTheme }
}
