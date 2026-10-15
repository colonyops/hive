import { ref } from 'vue'

export type ViewMode = 'list' | 'grid'

const storageKey = 'drive.view'

function load(): ViewMode {
  try {
    return localStorage.getItem(storageKey) === 'grid' ? 'grid' : 'list'
  } catch {
    return 'list'
  }
}

/** It lives in this browser only; storage can be blocked, so it is best effort. */
export function useViewMode() {
  const mode = ref<ViewMode>(load())
  function set(next: ViewMode): void {
    mode.value = next
    try {
      localStorage.setItem(storageKey, next)
    } catch {
      // The layout still applies for this page view.
    }
  }
  return { mode, set }
}
