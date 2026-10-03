import { ref, type Ref } from 'vue'
import { useTasks } from '../stores/useTasks'

/**
 * Tasks and Activity open over whatever is on screen rather than navigating
 * away from it (hay-kot/hive-desktop#441), so they are overlays, not routes.
 * Each title-bar icon toggles its overlay.
 */
export function useHubOverlays(terminalActive: Ref<boolean>) {
  const tasksOpen = ref(false)
  const activityOpen = ref(false)
  const { setRepoKey } = useTasks()
  // TerminalMode reports the attached session's owner/repo continuously, so
  // every way of opening Tasks scopes to it the same way.
  const terminalSessionRepoKey = ref('')

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

  return { tasksOpen, activityOpen, terminalSessionRepoKey, toggleTasks, toggleActivity }
}
