import { describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'

const setRepoKey = vi.hoisted(() => vi.fn())
vi.mock('../../stores/useTasks', () => ({ useTasks: () => ({ setRepoKey }) }))

import { useHubOverlays } from '../useHubOverlays'

describe('useHubOverlays', () => {
  it('scopes Tasks to the attached session only when opening it from terminal mode', () => {
    setRepoKey.mockClear()
    const terminalActive = ref(true)
    const overlays = useHubOverlays(terminalActive, ref(null))
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
    const overlays = useHubOverlays(ref(true), ref(null))

    overlays.toggleTasks()

    expect(overlays.tasksOpen.value).toBe(true)
    expect(setRepoKey).not.toHaveBeenCalled()
  })

  it('toggles Activity', () => {
    const overlays = useHubOverlays(ref(false), ref(null))
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(true)
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(false)
  })

  it('closes Changes when no session is left for it to show, and keeps it open across a switch', async () => {
    const session = ref<string | null>('a')
    const overlays = useHubOverlays(ref(true), session)
    overlays.toggleChanges()

    session.value = 'b'
    await nextTick()
    expect(overlays.changesOpen.value).toBe(true)

    session.value = null
    await nextTick()
    expect(overlays.changesOpen.value).toBe(false)
  })
})
