import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { CanvasScope } from '../../lib/agentCanvas'

const setRepoKey = vi.hoisted(() => vi.fn())
vi.mock('../../stores/useTasks', () => ({ useTasks: () => ({ setRepoKey }) }))

import { useHubOverlays } from '../useHubOverlays'

const noChat = ref<CanvasScope | null>(null)

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

  it("opens Canvases on the chat's canvas, and only an opening toggle moves the scope", () => {
    const chat = ref<CanvasScope | null>({ workspace: 'web-app', name: 'plan', session: 7 })
    const overlays = useHubOverlays(ref(false), chat)

    overlays.toggleCanvas()
    expect(overlays.canvasOpen.value).toBe(true)
    expect(overlays.canvasScope.value).toEqual({ workspace: 'web-app', name: 'plan', session: 7 })

    chat.value = { workspace: 'docs', name: null, session: 9 }
    overlays.toggleCanvas()
    expect(overlays.canvasOpen.value).toBe(false)
    expect(overlays.canvasScope.value.workspace).toBe('web-app')
  })

  it('reopens Canvases on the canvas last read when no chat is on screen', () => {
    const overlays = useHubOverlays(ref(false), noChat)

    overlays.openCanvas({ workspace: 'docs', name: 'perf-report', session: null })
    expect(overlays.canvasOpen.value).toBe(true)

    overlays.toggleCanvas()
    overlays.toggleCanvas()
    expect(overlays.canvasOpen.value).toBe(true)
    expect(overlays.canvasScope.value).toEqual({ workspace: 'docs', name: 'perf-report', session: null })
  })
})
