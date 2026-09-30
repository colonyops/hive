import { computed, ref, type ComputedRef, type Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetCanvasFontSize as PersistCanvasFontSize,
  SetCanvasLineSpacing as PersistCanvasLineSpacing,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

export const canvasFontSizes = ['small', 'medium', 'large', 'xl'] as const
export type CanvasFontSize = (typeof canvasFontSizes)[number]

export const canvasFontSizeLabels: Record<CanvasFontSize, string> = {
  small: 'Small',
  medium: 'Medium',
  large: 'Large',
  xl: 'Extra large',
}

export const canvasFontSizePx: Record<CanvasFontSize, number> = {
  small: 12,
  medium: 13.5,
  large: 15.5,
  xl: 18,
}

export const canvasLineSpacings = ['compact', 'standard', 'relaxed'] as const
export type CanvasLineSpacing = (typeof canvasLineSpacings)[number]

export const canvasLineSpacingLabels: Record<CanvasLineSpacing, string> = {
  compact: 'Compact',
  standard: 'Standard',
  relaxed: 'Relaxed',
}

export const canvasLineSpacingValues: Record<CanvasLineSpacing, number> = {
  compact: 1.45,
  standard: 1.65,
  relaxed: 1.85,
}

export const defaultCanvasFontSize: CanvasFontSize = 'medium'
export const defaultCanvasLineSpacing: CanvasLineSpacing = 'standard'

function isCanvasFontSize(value: string): value is CanvasFontSize {
  return canvasFontSizes.includes(value as CanvasFontSize)
}

function isCanvasLineSpacing(value: string): value is CanvasLineSpacing {
  return canvasLineSpacings.includes(value as CanvasLineSpacing)
}

const currentFontSize: Ref<CanvasFontSize> = ref(defaultCanvasFontSize)
const currentLineSpacing: Ref<CanvasLineSpacing> = ref(defaultCanvasLineSpacing)

let hydration: Promise<void> | null = null
let fontSizeVersion = 0
let lineSpacingVersion = 0
let persistChain: Promise<void> = Promise.resolve()

async function hydrate(): Promise<void> {
  const startedFontSize = fontSizeVersion
  const startedLineSpacing = lineSpacingVersion
  try {
    const settings = await GetAppearanceSettings()
    if (fontSizeVersion === startedFontSize && isCanvasFontSize(settings.canvasFontSize)) {
      currentFontSize.value = settings.canvasFontSize
    }
    if (lineSpacingVersion === startedLineSpacing && isCanvasLineSpacing(settings.canvasLineSpacing)) {
      currentLineSpacing.value = settings.canvasLineSpacing
    }
  } catch (error) {
    console.warn('Unable to load canvas typography from settings.yaml', error)
  }
}

function ensureHydrated(): Promise<void> {
  hydration ??= hydrate()
  return hydration
}

function persist(write: () => Promise<void>): void {
  persistChain = persistChain.then(write).catch((error: unknown) => {
    console.warn('Unable to persist canvas typography to settings.yaml', error)
  })
}

export function setCanvasFontSize(next: CanvasFontSize): void {
  fontSizeVersion++
  currentFontSize.value = next
  persist(() => PersistCanvasFontSize(next))
}

export function setCanvasLineSpacing(next: CanvasLineSpacing): void {
  lineSpacingVersion++
  currentLineSpacing.value = next
  persist(() => PersistCanvasLineSpacing(next))
}

export function useCanvasTypography(): {
  fontSize: Ref<CanvasFontSize>
  fontSizePx: ComputedRef<number>
  lineSpacing: Ref<CanvasLineSpacing>
  lineHeight: ComputedRef<number>
} {
  void ensureHydrated()
  return {
    fontSize: currentFontSize,
    fontSizePx: computed(() => canvasFontSizePx[currentFontSize.value]),
    lineSpacing: currentLineSpacing,
    lineHeight: computed(() => canvasLineSpacingValues[currentLineSpacing.value]),
  }
}

export function resetCanvasTypographyForTests(): void {
  currentFontSize.value = defaultCanvasFontSize
  currentLineSpacing.value = defaultCanvasLineSpacing
  hydration = null
  fontSizeVersion = 0
  lineSpacingVersion = 0
  persistChain = Promise.resolve()
}
