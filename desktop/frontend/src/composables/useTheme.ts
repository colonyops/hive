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

export function setTheme(nextTheme: Theme): void {
  document.documentElement.dataset.theme = nextTheme
  localStorage.setItem(storageKey, nextTheme)
}

// Called once before mount so the first paint uses the persisted theme.
export function initializeTheme(): void {
  const storedTheme = localStorage.getItem(storageKey)
  setTheme(isTheme(storedTheme) ? storedTheme : 'dark')
}
