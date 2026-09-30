import { useStorage } from '@vueuse/core'
import type { Ref } from 'vue'

// Recents are usage data, not configuration: device-local and rebuilt from
// behaviour, so they live in localStorage (hive.* pattern) rather than
// settings.yaml.
export const RECENT_LIMIT = 8

const storageKey = 'hive.palette.recents'

/**
 * Validates the stored shape on read so a value written by a future format,
 * or corrupted by hand, degrades to an empty list instead of feeding garbage
 * into the palette. JSON.parse failures degrade the same way.
 */
const recentsSerializer = {
  read: (raw: string): string[] => {
    try {
      const parsed: unknown = JSON.parse(raw)
      return Array.isArray(parsed) && parsed.every((entry) => typeof entry === 'string')
        ? parsed.slice(0, RECENT_LIMIT)
        : []
    } catch {
      return []
    }
  },
  write: (value: string[]): string => JSON.stringify(value),
}

// useStorage's own read/write already catch a throwing storage accessor
// (e.g. localStorage unavailable) and fall back to the initial value; onError
// here just keeps that failure on the codebase's console.warn convention
// instead of VueUse's default console.error.
const recentIds = useStorage<string[]>(storageKey, [], undefined, {
  serializer: recentsSerializer,
  onError: (error) => console.warn('Unable to read palette recents', error),
})

function recordRun(id: string): void {
  recentIds.value = [id, ...recentIds.value.filter((existing) => existing !== id)].slice(0, RECENT_LIMIT)
}

export function usePaletteRecents(): {
  /** Most-recent-first command ids, capped at RECENT_LIMIT. */
  recentIds: Ref<string[]>
  /** Move id to the front, dedupe, trim. */
  recordRun(id: string): void
} {
  return { recentIds, recordRun }
}

export function resetPaletteRecentsForTests(): void {
  recentIds.value = []
}
