import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetFontFamily: vi.fn(),
  SetMonoFontFamily: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  SetFontFamily: mocks.SetFontFamily,
  SetMonoFontFamily: mocks.SetMonoFontFamily,
}))

// The choices are module singletons that read the first-paint cache (via VueUse
// useStorage) at import time, so each test seeds localStorage first, then
// resets modules and imports fresh to exercise the startup path.
beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
  vi.clearAllMocks()
  mocks.AppearanceSettings.mockResolvedValue({ fontFamily: '', monoFontFamily: '' })
  mocks.SetFontFamily.mockResolvedValue(undefined)
  mocks.SetMonoFontFamily.mockResolvedValue(undefined)
})

afterEach(() => {
  document.documentElement.style.removeProperty('--font-sans')
  document.documentElement.style.removeProperty('--font-mono')
})

// Yielding to a macrotask drains the hydrate and persist chains without
// counting the microtask hops inside them.
async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

function sansVar(): string {
  return document.documentElement.style.getPropertyValue('--font-sans')
}

function monoVar(): string {
  return document.documentElement.style.getPropertyValue('--font-mono')
}

describe('useAppFont', () => {
  it('applies the cached choice synchronously, before the settings read resolves', async () => {
    localStorage.setItem('hive.font.sans', 'Iosevka')
    let resolveRead: (value: { fontFamily: string; monoFontFamily: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(new Promise((resolve) => { resolveRead = resolve }))
    const { initializeAppFont } = await import('../useAppFont')

    initializeAppFont()

    // No await: this is the first-paint guarantee.
    expect(sansVar()).toContain('"Iosevka"')
    resolveRead({ fontFamily: 'Iosevka', monoFontFamily: '' })
  })

  it('falls back to the bundled stacks when nothing has been chosen', async () => {
    const { initializeAppFont } = await import('../useAppFont')

    initializeAppFont()
    await settle()

    expect(sansVar()).toBe('"Inter Variable", system-ui, sans-serif')
    expect(monoVar()).toBe('"JetBrains Mono", ui-monospace, monospace')
  })

  it('prefers the durable settings choice over the cache', async () => {
    localStorage.setItem('hive.font.sans', 'Iosevka')
    mocks.AppearanceSettings.mockResolvedValue({ fontFamily: 'SF Pro Text', monoFontFamily: 'Fira Code' })
    const { initializeAppFont, useAppFont } = await import('../useAppFont')

    initializeAppFont()
    await settle()

    const { sans, mono } = useAppFont()
    expect(sans.value).toBe('SF Pro Text')
    expect(mono.value).toBe('Fira Code')
    expect(sansVar()).toContain('"SF Pro Text"')
  })

  // Empty is a real selection here — the bundled face — so settings.yaml wins
  // even when it holds nothing, or clearing a choice on one machine would be
  // undone by a stale cache on the next launch.
  it('lets an empty durable choice clear a cached one', async () => {
    localStorage.setItem('hive.font.sans', 'Iosevka')
    const { initializeAppFont, useAppFont } = await import('../useAppFont')

    initializeAppFont()
    await settle()

    expect(useAppFont().sans.value).toBe('')
    expect(sansVar()).toBe('"Inter Variable", system-ui, sans-serif')
  })

  it('caches a selection for the next first paint', async () => {
    const { initializeAppFont, setAppFontFamily } = await import('../useAppFont')
    initializeAppFont()
    await settle()

    setAppFontFamily('Iosevka')
    await settle()

    expect(localStorage.getItem('hive.font.sans')).toContain('Iosevka')
    expect(mocks.SetFontFamily).toHaveBeenCalledWith('Iosevka')
  })

  it('keeps the sans and mono choices independent', async () => {
    const { initializeAppFont, setAppMonoFontFamily, useAppFont } = await import('../useAppFont')
    initializeAppFont()
    await settle()

    setAppMonoFontFamily('Fira Code')
    await settle()

    expect(useAppFont().sans.value).toBe('')
    expect(monoVar()).toContain('"Fira Code"')
    expect(sansVar()).toBe('"Inter Variable", system-ui, sans-serif')
    expect(mocks.SetFontFamily).not.toHaveBeenCalled()
  })

  it('keeps the chosen family when persisting fails', async () => {
    mocks.SetFontFamily.mockRejectedValue(new Error('disk full'))
    const { initializeAppFont, setAppFontFamily, useAppFont } = await import('../useAppFont')
    initializeAppFont()
    await settle()

    setAppFontFamily('Iosevka')
    await settle()

    expect(useAppFont().sans.value).toBe('Iosevka')
    expect(sansVar()).toContain('"Iosevka"')
  })

  it('does not let a slow settings read clobber a choice made meanwhile', async () => {
    let resolveRead: (value: { fontFamily: string; monoFontFamily: string }) => void = () => {}
    mocks.AppearanceSettings.mockReturnValue(new Promise((resolve) => { resolveRead = resolve }))
    const { initializeAppFont, setAppFontFamily, useAppFont } = await import('../useAppFont')

    initializeAppFont()
    setAppFontFamily('Iosevka')
    resolveRead({ fontFamily: 'SF Pro Text', monoFontFamily: '' })
    await settle()

    expect(useAppFont().sans.value).toBe('Iosevka')
  })

  it('keeps the cached choice when the settings read fails', async () => {
    localStorage.setItem('hive.font.mono', 'Fira Code')
    mocks.AppearanceSettings.mockRejectedValue(new Error('binding unavailable'))
    const { initializeAppFont, useAppFont } = await import('../useAppFont')

    initializeAppFont()
    await settle()

    expect(useAppFont().mono.value).toBe('Fira Code')
    expect(monoVar()).toContain('"Fira Code"')
  })
})
