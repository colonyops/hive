import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, watch } from 'vue'
import { useTasks } from '../useTasks'

const mocks = vi.hoisted(() => ({
  ListTasks: vi.fn(),
  ReadTaskDetail: vi.fn(),
  SetTaskStatus: vi.fn(),
  DeleteTask: vi.fn(),
  PruneTasks: vi.fn(),
  TaskRepoKeys: vi.fn(),
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
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function task(id: string, overrides: Partial<{ status: string; repoKey: string; parentId: string }> = {}) {
  return {
    id,
    repoKey: overrides.repoKey ?? 'acme/site',
    epicId: '',
    parentId: overrides.parentId ?? '',
    sessionId: '',
    title: `Task ${id}`,
    type: 'task',
    status: overrides.status ?? 'open',
    blocked: false,
    depth: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  }
}

function detail(id: string) {
  return { ...task(id), desc: '', blockers: [], comments: [] }
}

function appError(kind: string, message = 'boom') {
  return Object.assign(new Error(message), { cause: { kind, message } })
}

describe('useTasks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.ListTasks.mockResolvedValue([])
    mocks.TaskRepoKeys.mockResolvedValue([])
    mocks.ReadTaskDetail.mockResolvedValue(detail('t1'))
  })

  function tasksUpdated(): void {
    const handler = mocks.On.mock.calls.find(([name]) => name === 'tasks:updated')?.[1] as () => void
    handler()
  }

  it('persists repo, filter and collapsed state under their storage keys', async () => {
    const tasks = useTasks()

    tasks.setRepoKey('acme/site')
    tasks.setFilter('active')
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

  it('reloads on tasks:updated while live, and not after it stops', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    const tasks = useTasks()

    tasks.startLiveUpdates()
    await flushPromises()
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1'])

    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    tasksUpdated()
    await flushPromises()
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1', 't2'])

    tasks.stopLiveUpdates()
    mocks.ListTasks.mockClear()
    tasksUpdated()
    await flushPromises()
    expect(mocks.ListTasks).not.toHaveBeenCalled()
  })

  it('keeps last-seen items and sets error when a reload fails', async () => {
    mocks.ListTasks.mockResolvedValueOnce([task('t1')]).mockRejectedValueOnce(appError('internal', 'temporary failure'))
    const tasks = useTasks()

    tasks.startLiveUpdates()
    await flushPromises()
    tasksUpdated()
    await flushPromises()
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1'])
    expect(tasks.error.value).toBe('temporary failure')

    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    tasksUpdated()
    await flushPromises()
    expect(tasks.error.value).toBeNull()
    expect(tasks.items.value.map((i) => i.id)).toEqual(['t1', 't2'])
  })

  it('clears a selection the freshly loaded list no longer contains', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detail('t1'))
    const tasks = useTasks()

    await tasks.reload()
    tasks.select('t1')
    await flushPromises()
    expect(tasks.selectedId.value).toBe('t1')

    // A repo-scope change swaps the list out from under the selection; the
    // cross-scope detail read itself still succeeds, so only the list
    // membership check can catch this.
    mocks.ListTasks.mockResolvedValue([task('t2', { repoKey: 'acme/other' })])
    tasks.setRepoKey('acme/other')
    await flushPromises()

    expect(tasks.selectedId.value).toBeNull()
    expect(tasks.detail.value).toBeNull()
  })

  it('drops a list response for a repo the user has already switched away from', async () => {
    let resolveStale: (items: ReturnType<typeof task>[]) => void = () => {}
    mocks.ListTasks.mockImplementationOnce(() => new Promise((resolve) => (resolveStale = resolve)))
    const tasks = useTasks()
    const seen: string[][] = []
    watch(tasks.items, (items) => seen.push(items.map((item) => item.id)), { flush: 'sync' })

    void tasks.reload()
    mocks.ListTasks.mockResolvedValue([task('n1', { repoKey: 'acme/new' })])
    tasks.setRepoKey('acme/new')
    await nextTick()
    resolveStale([task('o1')])
    await flushPromises()

    expect(seen).toEqual([['n1']])
  })

  it('loads detail on select and clears the selection when it is not found', async () => {
    mocks.ReadTaskDetail.mockResolvedValueOnce(detail('t1'))
    const tasks = useTasks()

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
    const tasks = useTasks()
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
    const tasks = useTasks()
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
    const tasks = useTasks()

    tasks.startLiveUpdates()
    await flushPromises()
    expect(tasks.repoKeys.value).toEqual(['acme/site'])

    mocks.TaskRepoKeys.mockResolvedValue(['acme/site', 'acme/other'])
    await tasks.reload()
    expect(tasks.repoKeys.value).toEqual(['acme/site', 'acme/other'])

    tasks.stopLiveUpdates()
  })
})
