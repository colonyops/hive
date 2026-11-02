import { computed, watchEffect, type Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetCodeScale as PersistCodeScale,
  SetDocumentScale as PersistDocumentScale,
  SetInterfaceScale as PersistInterfaceScale,
  SetTerminalScale as PersistTerminalScale,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { createLogger } from '../lib/log'
import { defineStore } from './defineStore'
import { usePersistedSetting, type PersistedSetting } from './usePersistedSetting'

const log = createLogger('surface-scale')

export const surfaces = ['interface', 'document', 'code', 'terminal'] as const
export type Surface = (typeof surfaces)[number]

export const surfaceLabels: Record<Surface, string> = {
  interface: 'Interface',
  document: 'Documents',
  code: 'Code',
  terminal: 'Terminal',
}

export const defaultScale = 100

// The bounds repeat settings.Min*Scale / Max*Scale, which govern the file
// (ADR text-sizes-are-per-surface-percentages).
export const surfaceScaleRanges: Record<Surface, { min: number; max: number }> = {
  interface: { min: 75, max: 150 },
  document: { min: 75, max: 200 },
  code: { min: 75, max: 200 },
  terminal: { min: 60, max: 500 },
}

const presets = [60, 75, 80, 90, 100, 110, 120, 130, 150, 175, 200, 300, 400, 500]

const terminalBasePx = 13
const documentBasePx = 13.5

export function clampScale(surface: Surface, percent: number): number {
  const { min, max } = surfaceScaleRanges[surface]
  return Math.min(max, Math.max(min, Math.round(percent)))
}

/** The next preset in that direction; a custom value goes to its nearest neighbour first. */
export function nextScale(surface: Surface, percent: number, direction: 1 | -1): number {
  const { min, max } = surfaceScaleRanges[surface]
  const rungs = presets.filter((preset) => preset >= min && preset <= max)
  const next = direction > 0 ? rungs.find((rung) => rung > percent) : rungs.findLast((rung) => rung < percent)
  return next ?? (direction > 0 ? max : min)
}

/** Whole pixels, because xterm rounds its cell to whole device pixels and a fraction only blurs it. */
export function terminalPx(percent: number): number {
  return Math.round((terminalBasePx * percent) / 100)
}

export function documentPx(percent: number): number {
  return Math.round(documentBasePx * percent) / 100
}

// settings.yaml cannot be read before the first paint, so the interface scale
// keeps a cache beside it the way the app's fonts do: without one, every launch
// at a non-default scale would visibly jump.
const interfaceCacheKey = 'hive.scale.interface'

function cachedInterfaceScale(): number {
  try {
    const cached = Number(localStorage.getItem(interfaceCacheKey))
    return cached ? clampScale('interface', cached) : defaultScale
  } catch {
    return defaultScale
  }
}

function cacheInterfaceScale(percent: number): void {
  try {
    localStorage.setItem(interfaceCacheKey, String(percent))
  } catch (error) {
    log.debug('unable to cache the interface scale', error)
  }
}

type Appearance = Awaited<ReturnType<typeof GetAppearanceSettings>>

export const useSurfaceScale = defineStore('surfaceScale', () => {
  let appearance: Promise<Appearance | null> | null = null
  const readAppearance = () => (appearance ??= loadAppearance())
  async function loadAppearance(): Promise<Appearance | null> {
    try {
      return await GetAppearanceSettings()
    } catch (error) {
      log.warn('unable to load the text sizes from settings.yaml', error)
      return null
    }
  }

  function setting(
    surface: Surface,
    read: (stored: Appearance) => number,
    write: (percent: number) => Promise<void>,
    initial = defaultScale,
  ): PersistedSetting<number> {
    return usePersistedSetting<number>({
      initial,
      read: async () => {
        const stored = await readAppearance()
        const percent = stored ? read(stored) : 0
        return percent > 0 ? clampScale(surface, percent) : undefined
      },
      write,
      label: `the ${surface} scale`,
    })
  }

  const settings: Record<Surface, PersistedSetting<number>> = {
    interface: setting(
      'interface',
      (stored) => stored.interfaceScale,
      (percent) => PersistInterfaceScale(percent),
      cachedInterfaceScale(),
    ),
    document: setting(
      'document',
      (stored) => stored.documentScale,
      (percent) => PersistDocumentScale(percent),
    ),
    code: setting(
      'code',
      (stored) => stored.codeScale,
      (percent) => PersistCodeScale(percent),
    ),
    terminal: setting(
      'terminal',
      (stored) => stored.terminalScale,
      (percent) => PersistTerminalScale(percent),
    ),
  }

  watchEffect(() => {
    const percent = settings.interface.value.value
    document.documentElement.style.setProperty('--interface-scale', String(percent / 100))
    cacheInterfaceScale(percent)
  })
  watchEffect(() => {
    document.documentElement.style.setProperty('--code-scale', String(settings.code.value.value / 100))
  })

  function setScale(surface: Surface, percent: number): void {
    const next = clampScale(surface, percent)
    if (next !== settings[surface].value.value) settings[surface].set(next)
  }

  // Waits for hydration because the step is relative: taken from the unhydrated
  // default it would write a neighbour of 100% over whatever settings.yaml holds.
  async function stepScale(surface: Surface, direction: 1 | -1): Promise<void> {
    await settings[surface].whenHydrated()
    setScale(surface, nextScale(surface, settings[surface].value.value, direction))
  }

  async function resetScale(surface: Surface): Promise<void> {
    await settings[surface].whenHydrated()
    setScale(surface, defaultScale)
  }

  const scales = Object.fromEntries(surfaces.map((surface) => [surface, settings[surface].value])) as Record<
    Surface,
    Readonly<Ref<number>>
  >

  return {
    scales,
    terminalPx: computed(() => terminalPx(scales.terminal.value)),
    documentPx: computed(() => documentPx(scales.document.value)),
    setScale,
    stepScale,
    resetScale,
  }
})
