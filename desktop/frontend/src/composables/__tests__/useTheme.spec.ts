import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

// currentTheme is a module singleton that loads the persisted theme (via VueUse
// useStorage) at import time, so each test sets localStorage first, then resets
// modules and imports fresh to exercise the startup-restore path.
beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
})

afterEach(() => {
  delete document.documentElement.dataset.theme
})

describe('useTheme', () => {
  it('restores the persisted theme at startup', async () => {
    localStorage.setItem('hive.theme', 'midnight')
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()

    expect(document.documentElement.dataset.theme).toBe('midnight')
  })

  it('falls back to dark for unknown stored themes', async () => {
    localStorage.setItem('hive.theme', 'solarized')
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()

    expect(document.documentElement.dataset.theme).toBe('dark')
    await nextTick()
    expect(localStorage.getItem('hive.theme')).toBe('dark')
  })

  it('defaults to dark and persists a selected theme', async () => {
    const { initializeTheme, setTheme } = await import('../useTheme')

    initializeTheme()
    expect(document.documentElement.dataset.theme).toBe('dark')

    setTheme('gruvbox')

    expect(document.documentElement.dataset.theme).toBe('gruvbox')
    await nextTick()
    expect(localStorage.getItem('hive.theme')).toBe('gruvbox')
  })
})
