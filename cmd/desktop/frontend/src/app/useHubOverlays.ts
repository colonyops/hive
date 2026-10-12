import { ref, type Ref } from 'vue'
import type { DriveScope } from '../lib/drive'
import { useTasks } from '../stores/useTasks'

/**
 * Tasks, Activity, Action runs and Drive open over whatever is on screen rather than
 * navigating away from it (hay-kot/hive-desktop#441), so they are overlays,
 * not routes. A title-bar icon or a command toggles each one.
 */
export function useHubOverlays(terminalActive: Ref<boolean>, viewDriveScope: Ref<DriveScope | null>) {
  const tasksOpen = ref(false)
  const activityOpen = ref(false)
  const driveOpen = ref(false)
  const actionRunsOpen = ref(false)
  const actionRunsRunId = ref<number | null>(null)
  const { setRepoKey } = useTasks()
  // TerminalMode reports the attached session's owner/repo continuously, so
  // every way of opening Tasks scopes to it the same way.
  const terminalSessionRepoKey = ref('')
  // Kept across a close, so a reopen from outside Chats returns to the
  // document last read.
  const driveScope = ref<DriveScope>({ space: '', path: null, session: null })

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

  // A plain toggle opens on the newest run rather than the last one linked.
  function toggleActionRuns(): void {
    if (!actionRunsOpen.value) actionRunsRunId.value = null
    actionRunsOpen.value = !actionRunsOpen.value
  }

  // Opening on a run selects it, even over an already-open viewer.
  function openActionRuns(runId?: number): void {
    if (runId) actionRunsRunId.value = runId
    actionRunsOpen.value = true
  }

  function openDrive(scope: DriveScope): void {
    driveScope.value = scope
    driveOpen.value = true
  }

  // Tasks' rule: only an opening toggle follows the chat or session on screen.
  function toggleDrive(): void {
    if (!driveOpen.value && viewDriveScope.value) driveScope.value = viewDriveScope.value
    driveOpen.value = !driveOpen.value
  }

  return {
    tasksOpen,
    activityOpen,
    driveOpen,
    driveScope,
    actionRunsOpen,
    actionRunsRunId,
    toggleActionRuns,
    openActionRuns,
    terminalSessionRepoKey,
    toggleTasks,
    toggleActivity,
    openDrive,
    toggleDrive,
  }
}
