import { ref, type Ref } from 'vue'
import type { UseTerminalWindows } from '../../composables/useTerminalWindows'
import type { TreeWindowRow } from './useTerminalTree'

export type WindowRename = ReturnType<typeof useWindowRename>

/** The inline rename of a live window row, applied to the session on screen. */
export function useWindowRename(session: Ref<UseTerminalWindows | null>) {
  const windowId = ref('')
  const draft = ref('')

  // The field sits inside the row, so a double-click meant for its text arrives
  // here as well; restarting the rename would discard what was typed.
  function start(win: TreeWindowRow): void {
    if (!win.live || windowId.value === win.windowId) return
    windowId.value = win.windowId
    draft.value = win.name
  }

  function commit(): void {
    const id = windowId.value
    if (!id) return
    windowId.value = ''
    const name = draft.value.trim()
    if (name) void session.value?.rename(id, name)
  }

  return { windowId, draft, start, commit }
}
