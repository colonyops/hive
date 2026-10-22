import { useStorage } from '@vueuse/core'
import { computed } from 'vue'

const defaults = { fontSizePx: 17, lineHeight: 1.65 }
const fontRange = { min: 12, max: 24 }

const finiteOr = (value: unknown, fallback: number): number => (Number.isFinite(value) ? Number(value) : fallback)

/** The override lives in this browser only; storage can be blocked, so it is best effort. */
export function useReaderPrefs() {
  const stored = useStorage<Partial<typeof defaults>>('drive.reader', { ...defaults }, undefined, {
    mergeDefaults: true,
  })
  const prefs = computed(() => ({
    fontSizePx: finiteOr(stored.value.fontSizePx, defaults.fontSizePx),
    lineHeight: finiteOr(stored.value.lineHeight, defaults.lineHeight),
  }))

  function resize(deltaPx: number): void {
    const next = Math.min(fontRange.max, Math.max(fontRange.min, prefs.value.fontSizePx + deltaPx))
    stored.value = { ...prefs.value, fontSizePx: next }
  }

  return { prefs, resize }
}
