import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { useSessionChanges, type DiffBase } from '../useSessionChanges'

const mocks = vi.hoisted(() => ({ SessionChanges: vi.fn() }))
const focused = ref(true)

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionChanges: mocks.SessionChanges,
}))
vi.mock('../../stores/useWindowFocus', () => ({ useWindowFocus: () => ({ focused }) }))

function answer(branch: string) {
  return {
    resolved: true,
    branch,
    defaultBranch: 'main',
    base: 'branch',
    branchAvailable: true,
    files: [],
    truncated: false,
  }
}

function setup(visible = ref(true)) {
  const sessionId = ref('1')
  const base = ref<DiffBase>('branch')
  const scope = effectScope()
  const api = scope.run(() => useSessionChanges(sessionId, base, () => visible.value))!
  return { api, sessionId, base, visible, scope }
}

describe('useSessionChanges', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    focused.value = true
    mocks.SessionChanges.mockImplementation((id: string) => Promise.resolve(answer(`branch-${id}`)))
  })
  afterEach(() => vi.useRealTimers())

  it('reads at once and every five seconds while visible', async () => {
    const { api, scope } = setup()
    await flushPromises()
    expect(api.changes.value?.branch).toBe('branch-1')
    expect(mocks.SessionChanges).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(5_000)
    expect(mocks.SessionChanges).toHaveBeenCalledTimes(2)
    scope.stop()
  })

  it('stops reading while hidden or blurred, and reads again on return', async () => {
    const { visible, scope } = setup()
    await flushPromises()
    mocks.SessionChanges.mockClear()

    visible.value = false
    await vi.advanceTimersByTimeAsync(10_000)
    expect(mocks.SessionChanges).not.toHaveBeenCalled()

    visible.value = true
    await flushPromises()
    expect(mocks.SessionChanges).toHaveBeenCalledTimes(1)

    mocks.SessionChanges.mockClear()
    focused.value = false
    await vi.advanceTimersByTimeAsync(10_000)
    expect(mocks.SessionChanges).not.toHaveBeenCalled()

    focused.value = true
    await flushPromises()
    expect(mocks.SessionChanges).toHaveBeenCalledTimes(1)
    scope.stop()
  })

  it('ignores an answer for the session it left', async () => {
    let finishFirst!: (value: unknown) => void
    mocks.SessionChanges.mockImplementationOnce(() => new Promise((resolve) => (finishFirst = resolve)))
    const { api, sessionId, scope } = setup()

    sessionId.value = '2'
    await flushPromises()
    expect(api.changes.value?.branch).toBe('branch-2')

    finishFirst(answer('stale'))
    await flushPromises()
    expect(api.changes.value?.branch).toBe('branch-2')
    scope.stop()
  })

  it("does not carry one session's files to the next", async () => {
    const { api, sessionId, scope } = setup()
    await flushPromises()
    mocks.SessionChanges.mockImplementationOnce(() => new Promise(() => {}))

    sessionId.value = '2'
    await flushPromises()
    expect(api.changes.value).toBeNull()
    scope.stop()
  })

  it('reads the new base when it changes and keeps the list meanwhile', async () => {
    const { api, base, scope } = setup()
    await flushPromises()
    mocks.SessionChanges.mockImplementationOnce(() => new Promise(() => {}))

    base.value = 'uncommitted'
    await flushPromises()
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('1', 'uncommitted')
    expect(api.changes.value?.branch).toBe('branch-1')
    scope.stop()
  })

  it('keeps the last good list beside a failed read, and clears the error on the next', async () => {
    const { api, scope } = setup()
    await flushPromises()

    mocks.SessionChanges.mockRejectedValueOnce(new Error('fatal: bad HEAD'))
    await vi.advanceTimersByTimeAsync(5_000)
    expect(api.error.value).toBe('fatal: bad HEAD')
    expect(api.changes.value?.branch).toBe('branch-1')

    await vi.advanceTimersByTimeAsync(5_000)
    expect(api.error.value).toBeNull()
    scope.stop()
  })
})
