import { useStorage } from '@vueuse/core'
import { computed } from 'vue'

export type ViewMode = 'list' | 'grid'

/** It lives in this browser only; storage can be blocked, so it is best effort. */
export function useViewMode() {
  const stored = useStorage<string>('drive.view', 'list')
  const mode = computed<ViewMode>(() => (stored.value === 'grid' ? 'grid' : 'list'))
  function set(next: ViewMode): void {
    stored.value = next
  }
  return { mode, set }
}
