import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, effectScope, ref } from 'vue'
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

function setup() {
  const sessionId = ref('1')
  const base = ref<DiffBase>('branch')
  const target = computed(() => ({ base: base.value, against: '', commit: '' }))
  const scope = effectScope()
  const api = scope.run(() => useSessionChanges(sessionId, target))!
  return { api, sessionId, base, scope }
}

describe('useSessionChanges', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    focused.value = true
    mocks.SessionChanges.mockImplementation((id: string) => Promise.resolve(answer(`branch-${id}`)))
  })
  afterEach(() => vi.useRealTimers())

  it('reads at once and every five seconds', async () => {
    const { api, scope } = setup()
    await flushPromises()
    expect(api.changes.value?.branch).toBe('branch-1')
    expect(mocks.SessionChanges).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(5_000)
    expect(mocks.SessionChanges).toHaveBeenCalledTimes(2)
    scope.stop()
  })

  it('stops polling when disposed during a read', async () => {
    const { scope } = setup()
    await flushPromises()
    let finish!: (value: unknown) => void
    mocks.SessionChanges.mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)))
    await vi.advanceTimersByTimeAsync(5_000)
    mocks.SessionChanges.mockClear()

    scope.stop()
    finish(answer('late'))
    await vi.advanceTimersByTimeAsync(30_000)

    expect(mocks.SessionChanges).not.toHaveBeenCalled()
  })

  it("drops the last target's list when the first read of a new one fails, but not when a poll does", async () => {
    const { api, base, scope } = setup()
    await flushPromises()

    mocks.SessionChanges.mockRejectedValueOnce(new Error('poll failed'))
    await vi.advanceTimersByTimeAsync(5_000)
    expect(api.changes.value?.branch).toBe('branch-1')

    mocks.SessionChanges.mockRejectedValueOnce(new Error('bad target'))
    base.value = 'staged'
    await flushPromises()
    expect(api.changes.value).toBeNull()
    expect(api.error.value).toBeTruthy()
    expect(api.pending.value).toBe(false)
    scope.stop()
  })

  it('stops reading while blurred, and reads again on return', async () => {
    const { scope } = setup()
    await flushPromises()
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
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('1', { base: 'uncommitted', against: '', commit: '' })
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
