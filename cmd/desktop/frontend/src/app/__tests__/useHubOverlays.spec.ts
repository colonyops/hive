import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const setRepoKey = vi.hoisted(() => vi.fn())
vi.mock('../../stores/useTasks', () => ({ useTasks: () => ({ setRepoKey }) }))

import { useHubOverlays } from '../useHubOverlays'

describe('useHubOverlays', () => {
  it('scopes Tasks to the attached session only when opening it from terminal mode', () => {
    setRepoKey.mockClear()
    const terminalActive = ref(true)
    const overlays = useHubOverlays(terminalActive)
    overlays.terminalSessionRepoKey.value = 'acme/site'

    overlays.toggleTasks()
    expect(overlays.tasksOpen.value).toBe(true)
    expect(setRepoKey).toHaveBeenCalledExactlyOnceWith('acme/site')

    overlays.toggleTasks()
    expect(overlays.tasksOpen.value).toBe(false)
    expect(setRepoKey).toHaveBeenCalledOnce()

    terminalActive.value = false
    overlays.toggleTasks()
    expect(setRepoKey).toHaveBeenCalledOnce()
  })

  it('leaves the persisted scope alone when the session has no resolved repo', () => {
    setRepoKey.mockClear()
    const overlays = useHubOverlays(ref(true))

    overlays.toggleTasks()

    expect(overlays.tasksOpen.value).toBe(true)
    expect(setRepoKey).not.toHaveBeenCalled()
  })

  it('toggles Activity', () => {
    const overlays = useHubOverlays(ref(false))
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(true)
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(false)
  })
})
