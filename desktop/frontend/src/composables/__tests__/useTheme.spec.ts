import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() { return values.size },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value),
  }
}

beforeEach(() => {
  vi.stubGlobal('localStorage', memoryStorage())
})

afterEach(() => {
  delete document.documentElement.dataset.theme
  vi.unstubAllGlobals()
  vi.resetModules()
})

describe('useTheme', () => {
  it('initializes from localStorage and applies the saved theme', async () => {
    localStorage.setItem('hive.theme', 'light')
    const { useTheme } = await import('../useTheme')

    const { theme } = useTheme()

    expect(theme.value).toBe('light')
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('defaults to dark and persists a toggled theme', async () => {
    const { useTheme } = await import('../useTheme')
    const { theme, toggleTheme } = useTheme()

    expect(theme.value).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')

    toggleTheme()

    expect(theme.value).toBe('light')
    expect(document.documentElement.dataset.theme).toBe('light')
    expect(localStorage.getItem('hive.theme')).toBe('light')
  })
})
