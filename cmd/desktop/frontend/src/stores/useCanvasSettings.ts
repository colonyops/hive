import { computed } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetCanvasLineSpacing as PersistCanvasLineSpacing,
  SetCanvasPageWidth as PersistCanvasPageWidth,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'
import { useSurfaceScale } from './useSurfaceScale'

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

export const canvasPageWidths = ['narrow', 'wide', 'full'] as const
export type CanvasPageWidth = (typeof canvasPageWidths)[number]

export const canvasPageWidthLabels: Record<CanvasPageWidth, string> = {
  narrow: 'Narrow',
  wide: 'Wide',
  full: 'Full',
}

// Full drops the limit but keeps the view's padding.
export const canvasPageWidthClasses: Record<CanvasPageWidth, string> = {
  narrow: 'max-w-3xl',
  wide: 'max-w-[1040px]',
  full: 'max-w-none',
}

export const defaultCanvasLineSpacing: CanvasLineSpacing = 'standard'
export const defaultCanvasPageWidth: CanvasPageWidth = 'narrow'

function isCanvasLineSpacing(value: string): value is CanvasLineSpacing {
  return canvasLineSpacings.includes(value as CanvasLineSpacing)
}

function isCanvasPageWidth(value: string): value is CanvasPageWidth {
  return canvasPageWidths.includes(value as CanvasPageWidth)
}

export const useCanvasSettings = defineStore('canvasSettings', () => {
  // Every setting comes from one read of settings.yaml.
  let appearance: ReturnType<typeof GetAppearanceSettings> | null = null
  const readAppearance = () => (appearance ??= GetAppearanceSettings())

  const lineSpacing = usePersistedSetting<CanvasLineSpacing>({
    initial: defaultCanvasLineSpacing,
    read: async () => {
      const stored = (await readAppearance()).canvasLineSpacing
      return isCanvasLineSpacing(stored) ? stored : undefined
    },
    write: (next) => PersistCanvasLineSpacing(next),
    label: 'the canvas line spacing',
  })
  const pageWidth = usePersistedSetting<CanvasPageWidth>({
    initial: defaultCanvasPageWidth,
    read: async () => {
      const stored = (await readAppearance()).canvasPageWidth
      return isCanvasPageWidth(stored) ? stored : undefined
    },
    write: (next) => PersistCanvasPageWidth(next),
    label: 'the canvas page width',
  })

  return {
    fontSizePx: useSurfaceScale().documentPx,
    lineSpacing: lineSpacing.value,
    lineHeight: computed(() => canvasLineSpacingValues[lineSpacing.value.value]),
    setLineSpacing: lineSpacing.set,
    pageWidth: pageWidth.value,
    pageWidthClass: computed(() => canvasPageWidthClasses[pageWidth.value.value]),
    setPageWidth: pageWidth.set,
  }
})
