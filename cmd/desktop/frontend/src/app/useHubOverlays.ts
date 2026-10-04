import { ref, type Ref } from 'vue'
import type { CanvasScope } from '../lib/agentCanvas'
import { useTasks } from '../stores/useTasks'

/**
 * Tasks, Activity and Canvases open over whatever is on screen rather than
 * navigating away from it (hay-kot/hive-desktop#441), so they are overlays,
 * not routes. A title-bar icon or a command toggles each one.
 */
export function useHubOverlays(terminalActive: Ref<boolean>, viewCanvasScope: Ref<CanvasScope | null>) {
  const tasksOpen = ref(false)
  const activityOpen = ref(false)
  const canvasOpen = ref(false)
  const { setRepoKey } = useTasks()
  // TerminalMode reports the attached session's owner/repo continuously, so
  // every way of opening Tasks scopes to it the same way.
  const terminalSessionRepoKey = ref('')
  // Kept across a close, so a reopen from outside Chats returns to the canvas
  // last read.
  const canvasScope = ref<CanvasScope>({ workspace: '', name: null, session: null })

  // Only an opening toggle re-resolves the scope. Closing never moves it, and
  // no resolved repo leaves the persisted last-picked scope alone.
  function toggleTasks(): void {
    if (!tasksOpen.value && terminalActive.value && terminalSessionRepoKey.value) {
      setRepoKey(terminalSessionRepoKey.value)
    }
    tasksOpen.value = !tasksOpen.value
  }

  function toggleActivity(): void {
    activityOpen.value = !activityOpen.value
  }

  function openCanvas(scope: CanvasScope): void {
    canvasScope.value = scope
    canvasOpen.value = true
  }

  // Tasks' rule: only an opening toggle follows the chat or session on screen.
  function toggleCanvas(): void {
    if (!canvasOpen.value && viewCanvasScope.value) canvasScope.value = viewCanvasScope.value
    canvasOpen.value = !canvasOpen.value
  }

  return {
    tasksOpen,
    activityOpen,
    canvasOpen,
    canvasScope,
    terminalSessionRepoKey,
    toggleTasks,
    toggleActivity,
    openCanvas,
    toggleCanvas,
  }
}
