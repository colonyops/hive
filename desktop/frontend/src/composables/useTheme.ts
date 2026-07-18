import { ref, type Ref } from 'vue'

export type Theme = 'dark' | 'light'

const storageKey = 'hive.theme'
const theme = ref<Theme>('dark')
let initialized = false

function applyTheme(nextTheme: Theme): void {
  theme.value = nextTheme
  document.documentElement.dataset.theme = nextTheme
  localStorage.setItem(storageKey, nextTheme)
}

function initializeTheme(): void {
  if (initialized) return

  const storedTheme = localStorage.getItem(storageKey)
  applyTheme(storedTheme === 'light' ? 'light' : 'dark')
  initialized = true
}

export function toggleTheme(): void {
  initializeTheme()
  applyTheme(theme.value === 'dark' ? 'light' : 'dark')
}

export function useTheme(): { theme: Ref<Theme>; toggleTheme: () => void } {
  initializeTheme()
  return { theme, toggleTheme }
}
