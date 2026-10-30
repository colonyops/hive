import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useModePreload } from '../useModePreload'

const mocks = vi.hoisted(() => ({
  probe: vi.fn(),
  reloadSessions: vi.fn(),
  reloadWorkspaces: vi.fn(),
  reloadRecents: vi.fn(),
  useTerminalShowWindows: vi.fn(),
  useCodeViews: vi.fn(),
  prepareCode: vi.fn(),
}))

vi.mock('../../components/TerminalMode.vue', () => ({ default: {} }))
vi.mock('../../components/AgentsMode.vue', () => ({ default: {} }))
vi.mock('../../stores/useTerminalAvailability', () => ({
  useTerminalAvailability: () => ({ probe: mocks.probe }),
}))
vi.mock('../../stores/useTerminalSessions', () => ({
  useTerminalSessions: () => ({ reload: mocks.reloadSessions }),
}))
vi.mock('../../stores/useAgentWorkspaces', () => ({
  useAgentWorkspaces: () => ({ reloadWorkspaces: mocks.reloadWorkspaces }),
}))
vi.mock('../../stores/useAgentSessionsAll', () => ({
  useAgentSessionsAll: () => ({ reload: mocks.reloadRecents }),
}))
vi.mock('../../stores/useTerminalShowWindows', () => ({ useTerminalShowWindows: mocks.useTerminalShowWindows }))
vi.mock('../../stores/useCodeViews', () => ({ useCodeViews: mocks.useCodeViews }))

describe('useModePreload', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  function setup(initial: boolean) {
    const ready = ref(initial)
    effectScope().run(() => useModePreload(() => ready.value, mocks.prepareCode))
    return ready
  }

  it('waits for the shell, then preloads once', async () => {
    const ready = setup(false)
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.probe).not.toHaveBeenCalled()

    ready.value = true
    await nextTick()
    expect(mocks.probe).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.probe).toHaveBeenCalledTimes(1)
    expect(mocks.reloadSessions).toHaveBeenCalledTimes(1)
    expect(mocks.reloadWorkspaces).toHaveBeenCalledTimes(1)
    expect(mocks.reloadRecents).toHaveBeenCalledTimes(1)
    await vi.waitFor(() => expect(mocks.prepareCode).toHaveBeenCalledTimes(1))

    ready.value = false
    await nextTick()
    ready.value = true
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.probe).toHaveBeenCalledTimes(1)
  })

  it('starts on its own when the shell is already up', async () => {
    setup(true)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.probe).toHaveBeenCalledTimes(1)
  })
})
