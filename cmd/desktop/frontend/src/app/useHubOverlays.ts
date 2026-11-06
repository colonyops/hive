import { ref } from 'vue'

/**
 * Activity and Action runs open over whatever is on screen rather than
 * navigating away from it (hay-kot/hive-desktop#441), so they are overlays,
 * not routes. A title-bar icon or a command toggles each one.
 */
export function useHubOverlays() {
  const activityOpen = ref(false)
  const actionRunsOpen = ref(false)
  const actionRunsRunId = ref<number | null>(null)
  // TerminalMode reports the attached session's owner/repo continuously, so
  // opening Tasks from Code scopes it to that session's repository.
  const terminalSessionRepoKey = ref('')

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

  return {
    activityOpen,
    actionRunsOpen,
    actionRunsRunId,
    toggleActionRuns,
    openActionRuns,
    terminalSessionRepoKey,
    toggleActivity,
  }
}
