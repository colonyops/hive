import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const mocks = vi.hoisted(() => ({
  ListTasks: vi.fn(),
  ReadTaskDetail: vi.fn(),
  SetTaskStatus: vi.fn(),
  DeleteTask: vi.fn(),
  PruneTasks: vi.fn(),
  TaskRepoKeys: vi.fn(),
  Focused: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/tasksservice', () => ({
  ListTasks: mocks.ListTasks,
  TaskDetail: mocks.ReadTaskDetail,
  SetTaskStatus: mocks.SetTaskStatus,
  DeleteTask: mocks.DeleteTask,
  PruneTasks: mocks.PruneTasks,
  TaskRepoKeys: mocks.TaskRepoKeys,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: mocks.Focused,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function task(id: string, overrides: Partial<{ status: string; repoKey: string; parentId: string }> = {}) {
  return {
    id, repoKey: overrides.repoKey ?? 'acme/site', epicId: '', parentId: overrides.parentId ?? '',
    sessionId: '', title: `Task ${id}`, type: 'task', status: overrides.status ?? 'open',
    blocked: false, depth: 0, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
  }
}

function detail(id: string) {
  return { ...task(id), desc: '', blockers: [], comments: [] }
}

function appError(kind: string, message = 'boom') {
  return Object.assign(new Error(message), { cause: { kind, message } })
}

async function loadComposable() {
  const { useTasks } = await import('../useTasks')
  return useTasks()
}

describe('useTasks', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.Focused.mockResolvedValue(true)
    mocks.ListTasks.mockResolvedValue([])
    mocks.TaskRepoKeys.mockResolvedValue([])
    mocks.ReadTaskDetail.mockResolvedValue(detail('t1'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // window:focus's real handler is registered by the (unmocked) useWindowFocus
  // module; grabbing it lets tests drive it the same way the native runtime would.
  function focusHandler(): () => void {
    return mocks.On.mock.calls.find(([name]) => name === 'window:focus')?.[1]
  }
  function blurHandler(): () => void {
    return mocks.On.mock.calls.find(([name]) => name === 'window:blur')?.[1]
  }

  it('persists repo, filter and collapsed state under their storage keys', async () => {
    const tasks = await loadComposable()

    tasks.repoKey.value = 'acme/site'
    tasks.filter.value = 'active'
    tasks.toggleCollapsed('epic-1')
    await nextTick()

    expect(localStorage.getItem('hive.tasks.repo')).toBe('acme/site')
    expect(localStorage.getItem('hive.tasks.filter')).toBe('active')
    expect(JSON.parse(localStorage.getItem('hive.tasks.collapsed') ?? '[]')).toEqual(['epic-1'])
    expect(tasks.isCollapsed('epic-1')).toBe(true)
    expect(tasks.isCollapsed('epic-2')).toBe(false)

    tasks.toggleCollapsed('epic-1')
    expect(tasks.isCollapsed('epic-1')).toBe(false)
  })

  it('polls on a self-rescheduling 2s timer and picks up external writes', async () => {
    vi.useFakeTimers()
    mocks.ListTasks.mockResolvedValue([task('t1')])
    const tasks = await loadComposable()

    tasks.startPolling()
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1'])

    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    await vi.advanceTimersByTimeAsync(1999)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(2)
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1', 't2'])

    tasks.stopPolling()
    await vi.advanceTimersByTimeAsync(5000)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(2)
  })

  it('keeps last-seen items, sets error, and keeps polling on a poll failure', async () => {
    vi.useFakeTimers()
    mocks.ListTasks
      .mockResolvedValueOnce([task('t1')])
      .mockRejectedValueOnce(appError('internal', 'temporary failure'))
    const tasks = await loadComposable()

    tasks.startPolling()
    await vi.advanceTimersByTimeAsync(0)
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1'])

    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(2)
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1'])
    expect(tasks.error.value).toBe('temporary failure')

    // The chain keeps running: the next tick's success clears the error.
    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    await vi.advanceTimersByTimeAsync(2000)
    expect(tasks.error.value).toBeNull()
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1', 't2'])
  })

  it('clears a selection the freshly loaded list no longer contains', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detail('t1'))
    const tasks = await loadComposable()

    await tasks.refresh()
    tasks.select('t1')
    await flushPromises()
    expect(tasks.selectedId.value).toBe('t1')

    // A repo-scope change swaps the list out from under the selection; the
    // cross-scope detail read itself still succeeds, so only the list
    // membership check can catch this.
    mocks.ListTasks.mockResolvedValue([task('t2', { repoKey: 'acme/other' })])
    tasks.repoKey.value = 'acme/other'
    await flushPromises()

    expect(tasks.selectedId.value).toBeNull()
    expect(tasks.detail.value).toBeNull()
  })

  it('loads detail on select and clears the selection when it is not found', async () => {
    mocks.ReadTaskDetail.mockResolvedValueOnce(detail('t1'))
    const tasks = await loadComposable()

    tasks.select('t1')
    await flushPromises()
    expect(tasks.selectedId.value).toBe('t1')
    expect(tasks.detail.value?.id).toBe('t1')

    mocks.ReadTaskDetail.mockRejectedValueOnce(appError('not_found', 'task not found'))
    tasks.select('t2')
    await flushPromises()

    expect(tasks.selectedId.value).toBeNull()
    expect(tasks.detail.value).toBeNull()
  })

  it('re-reads the list after setStatus and remove succeed', async () => {
    const tasks = await loadComposable()
    mocks.ListTasks.mockClear()
    mocks.ListTasks.mockResolvedValue([task('t1', { status: 'done' })])
    mocks.SetTaskStatus.mockResolvedValue(undefined)

    await tasks.setStatus('t1', 'done')

    expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'done')
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)
    expect(tasks.items.value[0].status).toBe('done')

    tasks.select('t1')
    await flushPromises()
    mocks.ListTasks.mockClear()
    mocks.ListTasks.mockResolvedValue([])
    mocks.DeleteTask.mockResolvedValue(undefined)

    await tasks.remove('t1')

    expect(mocks.DeleteTask).toHaveBeenCalledWith('t1')
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)
    // A successful remove of the selected item clears the selection.
    expect(tasks.selectedId.value).toBeNull()
    expect(tasks.detail.value).toBeNull()
  })

  it('runs a prune dry-run before the real prune, then re-reads', async () => {
    const tasks = await loadComposable()
    mocks.PruneTasks.mockResolvedValueOnce(7)

    const count = await tasks.pruneDryRun(30, 'acme/site')

    expect(count).toBe(7)
    expect(mocks.PruneTasks).toHaveBeenCalledWith(30, 'acme/site', true)

    mocks.ListTasks.mockClear()
    mocks.PruneTasks.mockResolvedValueOnce(7)

    await tasks.prune(30, 'acme/site')

    expect(mocks.PruneTasks).toHaveBeenLastCalledWith(30, 'acme/site', false)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)
  })

  it('reloads repo keys on start and on manual refresh', async () => {
    mocks.TaskRepoKeys.mockResolvedValue(['acme/site'])
    const tasks = await loadComposable()

    tasks.startPolling()
    await flushPromises()
    expect(tasks.repoKeys.value).toEqual(['acme/site'])

    mocks.TaskRepoKeys.mockResolvedValue(['acme/site', 'acme/other'])
    await tasks.refresh()
    expect(tasks.repoKeys.value).toEqual(['acme/site', 'acme/other'])

    tasks.stopPolling()
  })

  it('reloads on window focus only while polling is requested', async () => {
    vi.useFakeTimers()
    mocks.ListTasks.mockResolvedValue([task('t1')])
    const tasks = await loadComposable()

    tasks.startPolling()
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)

    // blur and focus are awaited separately so Vue's batched watcher sees
    // each transition rather than collapsing them into a same-tick no-op.
    mocks.ListTasks.mockClear()
    blurHandler()()
    await nextTick()
    focusHandler()()
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)

    // With no view open (stopPolling), refocusing must not fetch.
    tasks.stopPolling()
    mocks.ListTasks.mockClear()
    blurHandler()()
    await nextTick()
    focusHandler()()
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.ListTasks).not.toHaveBeenCalled()
  })
})
