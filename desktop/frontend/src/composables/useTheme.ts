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
const theme = ref<Theme>('dark')
let initialized = false

function isTheme(value: string | null): value is Theme {
  return themes.includes(value as Theme)
}

function applyTheme(nextTheme: Theme): void {
  theme.value = nextTheme
  document.documentElement.dataset.theme = nextTheme
  localStorage.setItem(storageKey, nextTheme)
}

function initializeTheme(): void {
  if (initialized) return

  const storedTheme = localStorage.getItem(storageKey)
  applyTheme(isTheme(storedTheme) ? storedTheme : 'dark')
  initialized = true
}

export function setTheme(nextTheme: Theme): void {
  initializeTheme()
  applyTheme(nextTheme)
}

export function useTheme(): { theme: Ref<Theme>; setTheme: (nextTheme: Theme) => void } {
  initializeTheme()
  return { theme, setTheme }
}
