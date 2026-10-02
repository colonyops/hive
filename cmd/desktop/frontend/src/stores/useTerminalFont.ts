import { computed } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalFontFamily as PersistTerminalFontFamily,
  SetTerminalFontSize as PersistTerminalFontSize,
  SetTerminalFontWeights as PersistTerminalFontWeights,
  SetTerminalLetterSpacing as PersistTerminalLetterSpacing,
  SetTerminalLineHeight as PersistTerminalLineHeight,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { TERMINAL_FONT } from '../lib/terminalFaces'
import { defineStore } from './defineStore'
import { usePersistedSetting, type PersistedSetting } from './usePersistedSetting'

// The bounds repeat settings.MinTerminalFontSizePx / MaxTerminalFontSizePx,
// which govern the file (ADR the-terminal-text-size-is-a-pixel-count).
export const minTerminalFontSizePx = 8
export const maxTerminalFontSizePx = 64
export const terminalFontSizeStepPx = 2
export const defaultTerminalFontSizePx = 13

// The weights the bundled face ships, which is what makes each one a distinct
// rendering rather than a label over the same outlines: CSS matches a requested
// weight to the nearest declared face and never synthesizes a lighter one. A
// system font with fewer faces collapses some of these together — that is the
// font's doing, not a bug here.
export const terminalFontWeights = [300, 350, 400, 600, 700] as const
export type TerminalFontWeight = (typeof terminalFontWeights)[number]

export const terminalFontWeightLabels: Record<TerminalFontWeight, string> = {
  300: 'Light',
  350: 'Semilight',
  400: 'Regular',
  600: 'Semibold',
  700: 'Bold',
}

// Semilight, half a step under Regular. Terminal panes rasterise through a
// canvas atlas (ADR terminal-atlas-renderer) and Canvas2D text does not inherit the
// `-webkit-font-smoothing: antialiased` the rest of the app is drawn with, so a
// face renders heavier here than the same face does in the DOM — which is what
// hay-kot/hive-desktop#181 reports as "everything is bold". Regular is what that issue was filed
// about and Light reads too thin at these sizes on a dark background, so the
// usable range is narrow and the default sits between them.
export const defaultTerminalFontWeight: TerminalFontWeight = 350
export const defaultTerminalFontWeightBold: TerminalFontWeight = 700

// Multiplies the cell height. Box drawing still meets the cell edges above 1:
// the atlas strokes a custom glyph across the padded cell and offsets it by
// exactly what the renderer centres the char box by, so the two cancel
// (ADR terminal-line-height-and-letter-spacing).
export const terminalLineHeights = [1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6] as const
export type TerminalLineHeight = (typeof terminalLineHeights)[number]

// Widens the cell by whole *device* pixels — xterm adds this to the device
// char width and rounds it, so on a 2x display a step is half a CSS pixel and
// a fractional setting would quantise to nothing.
export const terminalLetterSpacings = [0, 1, 2, 3] as const
export type TerminalLetterSpacing = (typeof terminalLetterSpacings)[number]

export const defaultTerminalLineHeight: TerminalLineHeight = 1.2
// Must stay 0: it doubles as the "nothing persisted" value in settings.yaml.
export const defaultTerminalLetterSpacing: TerminalLetterSpacing = 0

export function clampTerminalFontSize(px: number): number {
  return Math.min(maxTerminalFontSizePx, Math.max(minTerminalFontSizePx, Math.round(px)))
}

function isTerminalFontWeight(value: number | null): value is TerminalFontWeight {
  return terminalFontWeights.includes(value as TerminalFontWeight)
}

function isTerminalLineHeight(value: number | null): value is TerminalLineHeight {
  return terminalLineHeights.includes(value as TerminalLineHeight)
}

function isTerminalLetterSpacing(value: number | null): value is TerminalLetterSpacing {
  return terminalLetterSpacings.includes(value as TerminalLetterSpacing)
}

type Appearance = Awaited<ReturnType<typeof GetAppearanceSettings>>

// Shared by SettingsView's pickers and every open terminal. No first-paint
// cache: a terminal that opens before hydration lands at the defaults and the
// watchers in useTerminalWindows re-apply, so settings.yaml is the only store.
export const useTerminalFont = defineStore('terminalFont', () => {
  // One read hydrates every setting. An unavailable binding keeps the defaults
  // and warns once, not once per setting.
  let appearance: Promise<Appearance | null> | null = null
  const readAppearance = () => (appearance ??= loadAppearance())
  async function loadAppearance(): Promise<Appearance | null> {
    try {
      return await GetAppearanceSettings()
    } catch (error) {
      console.warn('Unable to load terminal font settings from settings.yaml', error)
      return null
    }
  }

  const px = usePersistedSetting<number>({
    initial: defaultTerminalFontSizePx,
    read: async () => {
      const stored = (await readAppearance())?.terminalFontSizePx ?? 0
      return stored > 0 ? clampTerminalFontSize(stored) : undefined
    },
    write: (next) => PersistTerminalFontSize(next),
    label: 'the terminal font size',
  })
  // Empty is the bundled face rather than a sentinel name, so a settings.yaml
  // written before this setting existed reads as "shipped default".
  const family = usePersistedSetting<string>({
    initial: '',
    read: async () => (await readAppearance())?.terminalFontFamily || undefined,
    write: (next) => PersistTerminalFontFamily(next),
    label: 'the terminal font family',
  })
  // Each weight's write carries the other, so both are annotated: inferred,
  // the pair is circular and TypeScript widens them to any.
  const weight: PersistedSetting<TerminalFontWeight> = usePersistedSetting({
    initial: defaultTerminalFontWeight,
    read: async () => {
      const stored = (await readAppearance())?.terminalFontWeight ?? null
      return isTerminalFontWeight(stored) ? stored : undefined
    },
    write: (next) => PersistTerminalFontWeights(next, weightBold.value.value),
    label: 'the terminal font weight',
  })
  const weightBold: PersistedSetting<TerminalFontWeight> = usePersistedSetting({
    initial: defaultTerminalFontWeightBold,
    read: async () => {
      const stored = (await readAppearance())?.terminalFontWeightBold ?? null
      return isTerminalFontWeight(stored) ? stored : undefined
    },
    write: (next) => PersistTerminalFontWeights(weight.value.value, next),
    label: 'the terminal bold font weight',
  })
  const lineHeight = usePersistedSetting<TerminalLineHeight>({
    initial: defaultTerminalLineHeight,
    read: async () => {
      const stored = (await readAppearance())?.terminalLineHeight ?? null
      return isTerminalLineHeight(stored) ? stored : undefined
    },
    write: (next) => PersistTerminalLineHeight(next),
    label: 'the terminal line height',
  })
  const letterSpacing = usePersistedSetting<TerminalLetterSpacing>({
    initial: defaultTerminalLetterSpacing,
    read: async () => {
      const stored = (await readAppearance())?.terminalLetterSpacing ?? null
      return isTerminalLetterSpacing(stored) ? stored : undefined
    },
    write: (next) => PersistTerminalLetterSpacing(next),
    label: 'the terminal letter spacing',
  })

  function setFontSize(value: number): void {
    const next = clampTerminalFontSize(value)
    if (next === px.value.value) return
    px.set(next)
  }

  // Waits for hydration because the step is relative: taken from the unhydrated
  // default it would write a neighbour of 13px over whatever settings.yaml holds.
  async function stepFontSize(delta: 1 | -1): Promise<void> {
    await px.whenHydrated()
    setFontSize(px.value.value + delta * terminalFontSizeStepPx)
  }

  // Before hydration the size already equals the default, so the write would be skipped.
  async function resetFontSize(): Promise<void> {
    await px.whenHydrated()
    setFontSize(defaultTerminalFontSizePx)
  }

  function setFontFamily(next: string): void {
    // The bundled face is stored as empty so it tracks the shipped font rather
    // than pinning today's name into a user's settings.yaml.
    family.set(next === TERMINAL_FONT ? '' : next)
  }

  /**
   * Every typography value that moves a cell's width or height, as one
   * comparable key. A size vote is only counted in the metrics it was measured
   * against, so a remembered one is keyed by this.
   */
  function cellMetrics(): string {
    return [
      px.value.value,
      family.value.value,
      weight.value.value,
      weightBold.value.value,
      lineHeight.value.value,
      letterSpacing.value.value,
    ].join('|')
  }

  return {
    px: px.value,
    family: family.value,
    /** The family to show selected: the bundled face stands in for empty. */
    selectedFamily: computed(() => family.value.value || TERMINAL_FONT),
    weight: weight.value,
    weightBold: weightBold.value,
    lineHeight: lineHeight.value,
    letterSpacing: letterSpacing.value,
    setFontSize,
    stepFontSize,
    resetFontSize,
    setFontFamily,
    setFontWeight: weight.set,
    setFontWeightBold: weightBold.set,
    setLineHeight: lineHeight.set,
    setLetterSpacing: letterSpacing.set,
    cellMetrics,
  }
})
