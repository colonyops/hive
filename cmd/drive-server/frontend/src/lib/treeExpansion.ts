import { ref } from 'vue'

const storageKey = 'drive.tree.expanded'

function load(): Record<string, boolean> {
  try {
    const stored = JSON.parse(localStorage.getItem(storageKey) ?? 'null') as Record<string, boolean> | null
    return stored && typeof stored === 'object' ? stored : {}
  } catch {
    return {}
  }
}

/** Fold state of one space's tree. It lives in this browser only; storage can be blocked, so it is best effort. */
export function useTreeExpansion(space: () => string) {
  const state = ref(load())
  const id = (dirPath: string) => `${space()}\n${dirPath}`

  function set(dirPath: string, open: boolean): void {
    state.value = { ...state.value, [id(dirPath)]: open }
    try {
      localStorage.setItem(storageKey, JSON.stringify(state.value))
    } catch {
      // The fold state still applies for this page view.
    }
  }

  return {
    isOpen: (dirPath: string) => state.value[id(dirPath)] === true,
    toggle: (dirPath: string) => set(dirPath, state.value[id(dirPath)] !== true),
    open: (dirPath: string) => {
      if (state.value[id(dirPath)] !== true) set(dirPath, true)
    },
  }
}
