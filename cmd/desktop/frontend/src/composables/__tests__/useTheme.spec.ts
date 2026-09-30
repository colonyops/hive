import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetTheme: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetTheme: mocks.SetTheme,
}))

// currentTheme is a module singleton that loads the cached theme (via VueUse
// useStorage) at import time, so each test sets localStorage first, then resets
// modules and imports fresh to exercise the startup path.
beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
  vi.clearAllMocks()
  // Default: nothing persisted durably yet.
  mocks.AppearanceSettings.mockResolvedValue({ theme: '', terminalFontSizePx: 13 })
  mocks.SetTheme.mockResolvedValue(undefined)
})

afterEach(() => {
  delete document.documentElement.dataset.theme
})

// Lets the hydrate/persist promise chains settle without coupling the test to
// the exact number of microtask hops inside them: yielding to a macrotask
// drains the whole microtask queue, however deeply the persist chain is nested.
async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

describe('useTheme', () => {
  it('applies the cached theme synchronously, before the settings read resolves', async () => {
    localStorage.setItem('hive.theme', 'midnight')
    let resolveRead: (value: { theme: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(new Promise((resolve) => { resolveRead = resolve }))
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()

    // No await: this is the first-paint guarantee.
    expect(document.documentElement.dataset.theme).toBe('midnight')
    resolveRead({ theme: 'midnight' })
  })

  it('prefers the durable settings theme over the cache', async () => {
    localStorage.setItem('hive.theme', 'dark')
    mocks.AppearanceSettings.mockResolvedValue({ theme: 'gruvbox' })
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()
    await settle()

    expect(document.documentElement.dataset.theme).toBe('gruvbox')
    expect(localStorage.getItem('hive.theme')).toBe('gruvbox')
  })

  it('restores the durable theme when the cache was wiped', async () => {
    // A new bundle id or dev-server port yields an empty localStorage.
    mocks.AppearanceSettings.mockResolvedValue({ theme: 'midnight' })
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()
    await settle()

    expect(document.documentElement.dataset.theme).toBe('midnight')
  })

  it('adopts a previously chosen theme when nothing is persisted yet', async () => {
    localStorage.setItem('hive.theme', 'gruvbox')
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()
    await settle()

    expect(mocks.SetTheme).toHaveBeenCalledWith('gruvbox')
    expect(document.documentElement.dataset.theme).toBe('gruvbox')
  })

  it('writes nothing on a first run with no prior choice', async () => {
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()
    await settle()

    // An absent key already means "use the default"; do not manufacture one.
    expect(mocks.SetTheme).not.toHaveBeenCalled()
    expect(document.documentElement.dataset.theme).toBe('dark')
  })

  it('heals an unrecognized persisted theme', async () => {
    localStorage.setItem('hive.theme', 'light')
    mocks.AppearanceSettings.mockResolvedValue({ theme: 'solarized' })
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()
    await settle()

    expect(document.documentElement.dataset.theme).toBe('light')
    expect(mocks.SetTheme).toHaveBeenCalledWith('light')
  })

  it('falls back to dark for unknown cached themes', async () => {
    localStorage.setItem('hive.theme', 'solarized')
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()

    expect(document.documentElement.dataset.theme).toBe('dark')
    await nextTick()
    expect(localStorage.getItem('hive.theme')).toBe('dark')
  })

  it('defaults to dark and persists a selected theme durably', async () => {
    const { initializeTheme, setTheme } = await import('../useTheme')

    initializeTheme()
    expect(document.documentElement.dataset.theme).toBe('dark')

    setTheme('gruvbox')

    expect(document.documentElement.dataset.theme).toBe('gruvbox')
    await nextTick()
    expect(localStorage.getItem('hive.theme')).toBe('gruvbox')
    await settle()
    expect(mocks.SetTheme).toHaveBeenCalledWith('gruvbox')
  })

  it('does not let a slow settings read clobber a theme picked meanwhile', async () => {
    localStorage.setItem('hive.theme', 'dark')
    let resolveRead: (value: { theme: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(new Promise((resolve) => { resolveRead = resolve }))
    const { initializeTheme, setTheme } = await import('../useTheme')

    initializeTheme()
    setTheme('light')
    resolveRead({ theme: 'gruvbox' })
    await settle()

    expect(document.documentElement.dataset.theme).toBe('light')
    expect(mocks.SetTheme).toHaveBeenCalledWith('light')
  })

  it('keeps the cached theme when the settings binding is unavailable', async () => {
    localStorage.setItem('hive.theme', 'midnight')
    mocks.AppearanceSettings.mockRejectedValue(new Error('no runtime'))
    const { initializeTheme } = await import('../useTheme')

    initializeTheme()
    await settle()

    expect(document.documentElement.dataset.theme).toBe('midnight')
  })

  it('keeps the applied theme when persisting fails', async () => {
    mocks.SetTheme.mockRejectedValue(new Error('disk full'))
    const { initializeTheme, setTheme } = await import('../useTheme')

    initializeTheme()
    setTheme('light')
    await settle()

    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('persists rapid selections in order', async () => {
    const { initializeTheme, setTheme } = await import('../useTheme')

    initializeTheme()
    setTheme('light')
    setTheme('midnight')
    setTheme('gruvbox')
    await settle()

    expect(mocks.SetTheme.mock.calls.map(([arg]) => arg)).toEqual([
      'light',
      'midnight',
      'gruvbox',
    ])
  })
})
