import { ref } from 'vue'

/**
 * Activity opens over whatever is on screen rather than navigating away from
 * it (hay-kot/hive-desktop#441), so it is an overlay, not a route. The
 * title-bar icon or its command toggles it.
 */
export function useHubOverlays() {
  const activityOpen = ref(false)

  function toggleActivity(): void {
    activityOpen.value = !activityOpen.value
  }

  return { activityOpen, toggleActivity }
}
