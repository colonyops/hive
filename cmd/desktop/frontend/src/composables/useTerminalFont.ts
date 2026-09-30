import { computed, readonly, ref, type ComputedRef, type Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalFontFamily as PersistTerminalFontFamily,
  SetTerminalFontSize as PersistTerminalFontSize,
  SetTerminalFontWeights as PersistTerminalFontWeights,
  SetTerminalLetterSpacing as PersistTerminalLetterSpacing,
  SetTerminalLineHeight as PersistTerminalLineHeight,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { TERMINAL_FONT } from '../lib/terminalFaces'

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
// #181 reports as "everything is bold". Regular is what that issue was filed
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

// Module singletons like useTheme's currentTheme, shared by SettingsView's
// pickers and every open terminal. No first-paint cache: a terminal that opens
// before hydration lands at the defaults and the watchers in useTerminalWindows
// re-apply, so the durable record in settings.yaml is the only store.
const currentSizePx: Ref<number> = ref(defaultTerminalFontSizePx)
// Empty is the bundled face rather than a sentinel name, so a settings.yaml
// written before this setting existed reads as "shipped default".
const currentFamily: Ref<string> = ref('')
const currentWeight: Ref<TerminalFontWeight> = ref(defaultTerminalFontWeight)
const currentWeightBold: Ref<TerminalFontWeight> = ref(defaultTerminalFontWeightBold)
const currentLineHeight: Ref<TerminalLineHeight> = ref(defaultTerminalLineHeight)
const currentLetterSpacing: Ref<TerminalLetterSpacing> = ref(defaultTerminalLetterSpacing)

let hydration: Promise<void> | null = null
// Same staleness guard as useTheme: a selection made while the hydrating read
// is in flight must not be overwritten by its result.
let version = 0
let persistChain: Promise<void> = Promise.resolve()

async function hydrate(): Promise<void> {
  const started = version
  try {
    const settings = await GetAppearanceSettings()
    if (version !== started) return
    if (settings.terminalFontSizePx > 0) {
      currentSizePx.value = clampTerminalFontSize(settings.terminalFontSizePx)
    }
    if (settings.terminalFontFamily) currentFamily.value = settings.terminalFontFamily
    if (isTerminalFontWeight(settings.terminalFontWeight)) {
      currentWeight.value = settings.terminalFontWeight
    }
    if (isTerminalFontWeight(settings.terminalFontWeightBold)) {
      currentWeightBold.value = settings.terminalFontWeightBold
    }
    if (isTerminalLineHeight(settings.terminalLineHeight)) {
      currentLineHeight.value = settings.terminalLineHeight
    }
    if (isTerminalLetterSpacing(settings.terminalLetterSpacing)) {
      currentLetterSpacing.value = settings.terminalLetterSpacing
    }
  } catch (error) {
    // An unavailable binding keeps the defaults; nothing to heal.
    console.warn('Unable to load terminal font settings from settings.yaml', error)
  }
}

function ensureHydrated(): Promise<void> {
  hydration ??= hydrate()
  return hydration
}

function persist(write: () => Promise<void>): void {
  version++
  // Chained so two quick selections cannot land out of order.
  persistChain = persistChain.then(write).catch((error: unknown) => {
    console.warn('Unable to persist terminal font settings to settings.yaml', error)
  })
}

export function setTerminalFontSize(px: number): void {
  const next = clampTerminalFontSize(px)
  if (next === currentSizePx.value) return
  currentSizePx.value = next
  persist(() => PersistTerminalFontSize(next))
}

// Waits for hydration because the step is relative: taken from the unhydrated
// default it would write a neighbour of 13px over whatever settings.yaml holds.
export async function stepTerminalFontSize(delta: 1 | -1): Promise<void> {
  await ensureHydrated()
  setTerminalFontSize(currentSizePx.value + delta * terminalFontSizeStepPx)
}

// Before hydration the size already equals the default, so the write would be skipped.
export async function resetTerminalFontSize(): Promise<void> {
  await ensureHydrated()
  setTerminalFontSize(defaultTerminalFontSizePx)
}

export function setTerminalFontFamily(next: string): void {
  // The bundled face is stored as empty so it tracks the shipped font rather
  // than pinning today's name into a user's settings.yaml.
  const family = next === TERMINAL_FONT ? '' : next
  currentFamily.value = family
  persist(() => PersistTerminalFontFamily(family))
}

export function setTerminalFontWeight(next: TerminalFontWeight): void {
  currentWeight.value = next
  persist(() => PersistTerminalFontWeights(next, currentWeightBold.value))
}

export function setTerminalFontWeightBold(next: TerminalFontWeight): void {
  currentWeightBold.value = next
  persist(() => PersistTerminalFontWeights(currentWeight.value, next))
}

export function setTerminalLineHeight(next: TerminalLineHeight): void {
  currentLineHeight.value = next
  persist(() => PersistTerminalLineHeight(next))
}

export function setTerminalLetterSpacing(next: TerminalLetterSpacing): void {
  currentLetterSpacing.value = next
  persist(() => PersistTerminalLetterSpacing(next))
}

/**
 * Every typography value that moves a cell's width or height, as one
 * comparable key. A size vote is only counted in the metrics it was measured
 * against, so a remembered one is keyed by this.
 */
export function terminalCellMetrics(): string {
  return [
    currentSizePx.value,
    currentFamily.value,
    currentWeight.value,
    currentWeightBold.value,
    currentLineHeight.value,
    currentLetterSpacing.value,
  ].join('|')
}

export function useTerminalFont(): {
  px: Readonly<Ref<number>>
  family: Ref<string>
  /** The family to show selected: the bundled face stands in for empty. */
  selectedFamily: ComputedRef<string>
  weight: Ref<TerminalFontWeight>
  weightBold: Ref<TerminalFontWeight>
  lineHeight: Ref<TerminalLineHeight>
  letterSpacing: Ref<TerminalLetterSpacing>
} {
  void ensureHydrated()
  return {
    px: readonly(currentSizePx),
    family: currentFamily,
    selectedFamily: computed(() => currentFamily.value || TERMINAL_FONT),
    weight: currentWeight,
    weightBold: currentWeightBold,
    lineHeight: currentLineHeight,
    letterSpacing: currentLetterSpacing,
  }
}

export function resetTerminalFontForTests(): void {
  currentSizePx.value = defaultTerminalFontSizePx
  currentFamily.value = ''
  currentWeight.value = defaultTerminalFontWeight
  currentWeightBold.value = defaultTerminalFontWeightBold
  currentLineHeight.value = defaultTerminalLineHeight
  currentLetterSpacing.value = defaultTerminalLetterSpacing
  hydration = null
  version = 0
  persistChain = Promise.resolve()
}
