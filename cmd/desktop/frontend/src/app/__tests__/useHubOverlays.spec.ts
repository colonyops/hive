import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { DriveScope } from '../../lib/drive'

const setRepoKey = vi.hoisted(() => vi.fn())
vi.mock('../../stores/useTasks', () => ({ useTasks: () => ({ setRepoKey }) }))

import { useHubOverlays } from '../useHubOverlays'

const noChat = ref<DriveScope | null>(null)

describe('useHubOverlays', () => {
  it('scopes Tasks to the attached session only when opening it from terminal mode', () => {
    setRepoKey.mockClear()
    const terminalActive = ref(true)
    const overlays = useHubOverlays(terminalActive, noChat)
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
    const overlays = useHubOverlays(ref(true), noChat)

    overlays.toggleTasks()

    expect(overlays.tasksOpen.value).toBe(true)
    expect(setRepoKey).not.toHaveBeenCalled()
  })

  it('toggles Activity', () => {
    const overlays = useHubOverlays(ref(false), noChat)
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(true)
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(false)
  })

  it("opens Drive on the chat's document, and only an opening toggle moves the scope", () => {
    const chat = ref<DriveScope | null>({ space: 'workspaces/web-app', path: 'plans/x.md', session: '7' })
    const overlays = useHubOverlays(ref(false), chat)

    overlays.toggleDrive()
    expect(overlays.driveOpen.value).toBe(true)
    expect(overlays.driveScope.value).toEqual({ space: 'workspaces/web-app', path: 'plans/x.md', session: '7' })

    chat.value = { space: 'workspaces/docs', path: null, session: '9' }
    overlays.toggleDrive()
    expect(overlays.driveOpen.value).toBe(false)
    expect(overlays.driveScope.value.space).toBe('workspaces/web-app')
  })

  it('reopens Drive on the document last read when no chat is on screen', () => {
    const overlays = useHubOverlays(ref(false), noChat)

    overlays.openDrive({ space: 'shared', path: 'canvases/perf-report.canvas.json', session: null })
    expect(overlays.driveOpen.value).toBe(true)

    overlays.toggleDrive()
    overlays.toggleDrive()
    expect(overlays.driveOpen.value).toBe(true)
    expect(overlays.driveScope.value).toEqual({
      space: 'shared',
      path: 'canvases/perf-report.canvas.json',
      session: null,
    })
  })
})
