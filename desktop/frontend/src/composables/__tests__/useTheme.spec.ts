import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initializeTheme, setTheme } from '../useTheme'

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
})

describe('useTheme', () => {
  it('initializes from localStorage and applies the saved theme', () => {
    localStorage.setItem('hive.theme', 'midnight')

    initializeTheme()

    expect(document.documentElement.dataset.theme).toBe('midnight')
  })

  it('falls back to dark for unknown stored themes', () => {
    localStorage.setItem('hive.theme', 'solarized')

    initializeTheme()

    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(localStorage.getItem('hive.theme')).toBe('dark')
  })

  it('defaults to dark and persists a selected theme', () => {
    initializeTheme()

    expect(document.documentElement.dataset.theme).toBe('dark')

    setTheme('gruvbox')

    expect(document.documentElement.dataset.theme).toBe('gruvbox')
    expect(localStorage.getItem('hive.theme')).toBe('gruvbox')
  })
})
