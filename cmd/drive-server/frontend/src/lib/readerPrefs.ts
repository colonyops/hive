import { ref } from 'vue'

const key = 'drive.reader'
const defaults = { fontSizePx: 17, lineHeight: 1.65 }
const fontRange = { min: 12, max: 24 }

function load(): typeof defaults {
  try {
    const stored = JSON.parse(localStorage.getItem(key) ?? 'null') as Partial<typeof defaults> | null
    return {
      fontSizePx: Number.isFinite(stored?.fontSizePx) ? Number(stored?.fontSizePx) : defaults.fontSizePx,
      lineHeight: Number.isFinite(stored?.lineHeight) ? Number(stored?.lineHeight) : defaults.lineHeight,
    }
  } catch {
    return { ...defaults }
  }
}

/** Reader typography. The override lives in this browser only; storage can be blocked, so it is best effort. */
export function useReaderPrefs() {
  const prefs = ref(load())

  function resize(deltaPx: number): void {
    const next = Math.min(fontRange.max, Math.max(fontRange.min, prefs.value.fontSizePx + deltaPx))
    prefs.value = { ...prefs.value, fontSizePx: next }
    try {
      localStorage.setItem(key, JSON.stringify(prefs.value))
    } catch {
      // The size still applies for this page view.
    }
  }

  return { prefs, resize }
}
