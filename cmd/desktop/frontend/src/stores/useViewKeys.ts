import { readonly, ref } from 'vue'
import { defineStore } from './defineStore'

/**
 * Whether the tab in front of Code's focused panel is a view with keys of its
 * own (Diff, Tasks), with nothing over it and the tree not focused. The keymap
 * then leaves bare keys to the view, as it leaves them to a focused pane.
 */
export const useViewKeys = defineStore('viewKeys', () => {
  const owned = ref(false)

  function setOwned(next: boolean): void {
    owned.value = next
  }

  return { owned: readonly(owned), setOwned }
})
