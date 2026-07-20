import { ref, type Ref } from 'vue'

export const themes = ['dark', 'light', 'midnight', 'gruvbox'] as const
export type Theme = (typeof themes)[number]

export const themeLabels: Record<Theme, string> = {
  dark: 'Dark',
  light: 'Light',
  midnight: 'Midnight',
  gruvbox: 'Gruvbox',
}

const storageKey = 'hive.theme'

function isTheme(value: string | null): value is Theme {
  return themes.includes(value as Theme)
}

// Reactive mirror of document.documentElement.dataset.theme — a module
// singleton (like useFlowsSession's shared session) so every caller (the
// command palette's theme:* commands, SettingsView's Appearance section)
// reads/drives the same live value instead of each holding a stale local
// copy. Starts at the setTheme() default; initializeTheme() (called once in
// main.ts before mount) overwrites it with the persisted value.
const currentTheme: Ref<Theme> = ref('dark')

export function setTheme(nextTheme: Theme): void {
  document.documentElement.dataset.theme = nextTheme
  localStorage.setItem(storageKey, nextTheme)
  currentTheme.value = nextTheme
}

// Called once before mount so the first paint uses the persisted theme.
export function initializeTheme(): void {
  const storedTheme = localStorage.getItem(storageKey)
  setTheme(isTheme(storedTheme) ? storedTheme : 'dark')
}

/** The live theme, kept in sync by every setTheme()/initializeTheme() call. */
export function useTheme(): { theme: Ref<Theme> } {
  return { theme: currentTheme }
}
