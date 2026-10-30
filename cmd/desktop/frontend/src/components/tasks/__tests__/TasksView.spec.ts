import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { chooseOption, openSelect } from '@hive/ui/test-utils/select'

const mocks = vi.hoisted(() => ({
  ListTasks: vi.fn(),
  ReadTaskDetail: vi.fn(),
  SetTaskStatus: vi.fn(),
  DeleteTask: vi.fn(),
  PruneTasks: vi.fn(),
  TaskRepoKeys: vi.fn(),
  // useTasks.ts calls useWindowFocus() at module scope, which fires this the
  // instant the test file's imports are evaluated — before any beforeEach has
  // a chance to configure it. clearAllMocks() (unlike resetAllMocks()) keeps
  // this default resolved value across tests, so it only needs setting once.
  On: vi.fn(() => () => {}),
  SetText: vi.fn(),
  OpenURL: vi.fn(),
}))

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/tasksservice', () => ({
  ListTasks: mocks.ListTasks,
  TaskDetail: mocks.ReadTaskDetail,
  SetTaskStatus: mocks.SetTaskStatus,
  DeleteTask: mocks.DeleteTask,
  PruneTasks: mocks.PruneTasks,
  TaskRepoKeys: mocks.TaskRepoKeys,
}))
vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Clipboard: { SetText: mocks.SetText },
  Browser: { OpenURL: mocks.OpenURL },
}))

import TasksView from '../TasksView.vue'
import { useTasks } from '../../../stores/useTasks'
import { resetToastsForTests, useToasts } from '../../../composables/useToasts'

interface TaskOverrides {
  status: string
  type: string
  parentId: string
  blocked: boolean
  sessionId: string
  repoKey: string
  title: string
  updatedAt: string
}

function task(id: string, overrides: Partial<TaskOverrides> = {}) {
  return {
    id,
    repoKey: overrides.repoKey ?? 'acme/site',
    epicId: '',
    parentId: overrides.parentId ?? '',
    sessionId: overrides.sessionId ?? '',
    title: overrides.title ?? `Task ${id}`,
    type: overrides.type ?? 'task',
    status: overrides.status ?? 'open',
    blocked: overrides.blocked ?? false,
    depth: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: overrides.updatedAt ?? '2026-01-01T00:00:00Z',
  }
}

function detailFrom(
  item: ReturnType<typeof task>,
  patch: Partial<{ desc: string; blockers: unknown[]; comments: unknown[] }> = {},
) {
  return { ...item, desc: patch.desc ?? '', blockers: patch.blockers ?? [], comments: patch.comments ?? [] }
}

async function selectRow(wrapper: ReturnType<typeof mount>, id: string): Promise<void> {
  await wrapper.get(`[data-testid="task-row"][data-id="${id}"]`).trigger('click')
  await flushPromises()
}

describe('TasksView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetToastsForTests()
    mocks.On.mockReturnValue(() => {})
    mocks.ListTasks.mockResolvedValue([])
    mocks.TaskRepoKeys.mockResolvedValue([])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    mocks.SetTaskStatus.mockResolvedValue(undefined)
    mocks.DeleteTask.mockResolvedValue(undefined)
    mocks.SetText.mockResolvedValue(undefined)
  })

  it('renders the tree from mocked items, indenting a child under its epic', async () => {
    mocks.ListTasks.mockResolvedValue([task('e1', { type: 'epic' }), task('t1', { parentId: 'e1' })])
    const wrapper = mount(TasksView)
    await flushPromises()

    const rows = wrapper.findAll('[data-testid="task-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('Task e1')
    expect(rows[1].text()).toContain('Task t1')
    expect(rows[0].attributes('data-id')).toBe('e1')
    expect(rows[1].attributes('data-id')).toBe('t1')
  })

  it('shows the no-data hint when the repo has no tasks at all', async () => {
    const wrapper = mount(TasksView)
    await flushPromises()

    expect(wrapper.find('[data-testid="tasks-empty"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('hive hc create')
  })

  it('shows a distinct empty state when items exist but none match the active filter', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1', { status: 'done' })])
    const wrapper = mount(TasksView)
    await flushPromises()

    // Default filter is "open"; a done-only item set matches nothing under it.
    expect(wrapper.find('[data-testid="tasks-empty-filter"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="tasks-empty"]').exists()).toBe(false)
  })

  it('persists the active filter under its storage key', async () => {
    const wrapper = mount(TasksView)
    await flushPromises()

    await wrapper.get('[data-testid="tasks-filter-all"]').trigger('click')

    expect(localStorage.getItem('hive.tasks.filter')).toBe('all')
    expect(wrapper.get('[data-testid="tasks-filter-all"]').attributes('aria-pressed')).toBe('true')
  })

  it('re-reads the list after a status change that needs no confirm', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    mocks.ListTasks.mockClear()
    await chooseOption(wrapper, 'task-status-select', 'in_progress')
    await flushPromises()

    expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'in_progress')
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)
  })

  it('confirms cancelling a task before applying it', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    await chooseOption(wrapper, 'task-status-select', 'cancelled')
    await flushPromises()

    expect(mocks.SetTaskStatus).not.toHaveBeenCalled()
    const confirm = document.querySelector('[data-testid="task-cancel-confirm"]')
    expect(confirm).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="task-cancel-confirm-confirm"]')!.click()
    await flushPromises()
    expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'cancelled')
  })

  it('confirms an epic status cascade, naming the open descendant count', async () => {
    const epic = task('e1', { type: 'epic' })
    mocks.ListTasks.mockResolvedValue([
      epic,
      task('c1', { parentId: 'e1', status: 'open' }),
      task('c2', { parentId: 'e1', status: 'in_progress' }),
      task('c3', { parentId: 'e1', status: 'done' }),
    ])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(epic))
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 'e1')

    await chooseOption(wrapper, 'task-status-select', 'done')
    await flushPromises()

    expect(mocks.SetTaskStatus).not.toHaveBeenCalled()
    const confirm = document.querySelector('[data-testid="task-cascade-confirm"]')
    // Two open/in_progress children count toward the cascade; the done one does not.
    expect(confirm?.textContent).toContain('also closes 2 open tasks')

    document.querySelector<HTMLButtonElement>('[data-testid="task-cascade-confirm-confirm"]')!.click()
    await flushPromises()
    expect(mocks.SetTaskStatus).toHaveBeenCalledWith('e1', 'done')
  })

  it('warns that delete removes the whole subtree, then re-reads after confirming', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    await wrapper.get('[data-testid="task-delete"]').trigger('click')
    const confirm = document.querySelector('[data-testid="task-delete-confirm"]')
    expect(confirm?.textContent).toContain('everything nested under it')
    expect(confirm?.textContent).toContain('comments included')

    mocks.ListTasks.mockClear()
    mocks.ListTasks.mockResolvedValue([])
    document.querySelector<HTMLButtonElement>('[data-testid="task-delete-confirm-confirm"]')!.click()
    await flushPromises()

    expect(mocks.DeleteTask).toHaveBeenCalledWith('t1')
    expect(mocks.ListTasks).toHaveBeenCalledTimes(1)
  })

  it('shows the prune dry-run count in the confirm copy before pruning', async () => {
    mocks.PruneTasks.mockResolvedValueOnce(5)
    const wrapper = mount(TasksView)
    await flushPromises()

    await wrapper.get('[data-testid="tasks-prune"]').trigger('click')
    await flushPromises()

    expect(mocks.PruneTasks).toHaveBeenCalledWith(30, '', true)
    const confirm = document.querySelector('[data-testid="tasks-prune-confirm"]')
    expect(confirm?.textContent).toContain('5 tasks')
    expect(confirm?.textContent).toContain('nested under a pruned root')

    mocks.PruneTasks.mockResolvedValueOnce(0)
    document.querySelector<HTMLButtonElement>('[data-testid="tasks-prune-confirm-confirm"]')!.click()
    await flushPromises()
    expect(mocks.PruneTasks).toHaveBeenLastCalledWith(30, '', false)
  })

  it('surfaces a dry-run failure in a banner, since it never opens the confirm dialog', async () => {
    mocks.PruneTasks.mockRejectedValueOnce(
      Object.assign(new Error('boom'), { cause: { kind: 'internal', message: 'hc store is locked' } }),
    )
    const wrapper = mount(TasksView)
    await flushPromises()

    await wrapper.get('[data-testid="tasks-prune"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="tasks-prune-confirm"]')).toBeNull()
    expect(wrapper.get('[data-testid="tasks-prune-error"]').text()).toContain('hc store is locked')
  })

  it('surfaces a load failure in the error banner, never the error dialog', async () => {
    mocks.ListTasks.mockRejectedValue(
      Object.assign(new Error('boom'), { cause: { kind: 'unavailable', message: 'hc store is not running' } }),
    )
    const wrapper = mount(TasksView)
    await flushPromises()

    expect(wrapper.get('[data-testid="tasks-error"]').text()).toContain('hc store is not running')
    expect(wrapper.find('[data-testid="error-dialog"]').exists()).toBe(false)
  })

  it('copies the task id via the native clipboard, flipping the button to Copied', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')
    expect(wrapper.get('[data-testid="task-detail-copy-id"]').text()).toBe('t1')

    await wrapper.get('[data-testid="task-detail-copy-id"]').trigger('click')
    await flushPromises()

    expect(mocks.SetText).toHaveBeenCalledWith('t1')
    expect(wrapper.get('[data-testid="task-detail-copy-id"]').text()).toBe('Copied')
  })

  it('renders a vanished blocker by its bare id, and a checkpoint comment with its badge and stripped prefix', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(
      detailFrom(task('t1'), {
        blockers: [
          { id: 'b1', title: 'Ship the API', status: 'open' },
          { id: 'b2', title: '', status: '' },
        ],
        comments: [{ id: 'c1', message: 'CHECKPOINT: landed the migration', createdAt: '2026-01-01T00:00:00Z' }],
      }),
    )
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    const blockers = wrapper.findAll('[data-testid="task-blocker-chip"]')
    expect(blockers[0].text()).toContain('Ship the API')
    expect(blockers[1].text()).toContain('b2')

    expect(wrapper.find('[data-testid="task-comment-checkpoint"]').exists()).toBe(true)
    const comment = wrapper.get('[data-testid="task-comment"]')
    expect(comment.text()).toContain('landed the migration')
    expect(comment.text()).not.toContain('CHECKPOINT:')
  })

  it('emits close from the header close button', async () => {
    const wrapper = mount(TasksView)
    await flushPromises()

    await wrapper.get('[data-testid="tasks-close"]').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('auto-selects the first visible row once the list first loads, but never steals an existing selection on a later reload', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()

    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')

    await selectRow(wrapper, 't2')
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t2')

    await wrapper.get('[data-testid="tasks-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t2')

    wrapper.unmount()
  })

  it('selects the first row of the new list when a repo switch drops the selection', async () => {
    mocks.TaskRepoKeys.mockResolvedValue(['acme/site', 'acme/other'])
    mocks.ListTasks.mockImplementation((key: string) =>
      Promise.resolve(
        key === 'acme/other'
          ? [task('x1', { repoKey: 'acme/other' }), task('x2', { repoKey: 'acme/other' })]
          : [task('t1'), task('t2')],
      ),
    )
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')

    await chooseOption(wrapper, 'tasks-repo-select', 'acme/other')
    await flushPromises()

    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task x1')

    wrapper.unmount()
  })

  it('moves the tree selection with j/k and the arrow keys, without wrapping past either end', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2'), task('t3')])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')

    async function press(key: string): Promise<void> {
      window.dispatchEvent(new KeyboardEvent('keydown', { key }))
      await flushPromises()
    }

    await press('j')
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t2')
    await press('ArrowDown')
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t3')
    await press('j') // no wrap past the last row
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t3')

    await press('k')
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t2')
    await press('ArrowUp')
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')
    await press('k') // no wrap past the first row
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')

    wrapper.unmount()
  })

  it('copies the selected row id to the clipboard with y and confirms it with a toast', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }))
    await flushPromises()
    expect(mocks.SetText).toHaveBeenCalledWith('t1')

    const { toasts } = useToasts()
    expect(toasts.value).toHaveLength(1)
    expect(toasts.value[0].message).toBe('Copied t1')
    expect(toasts.value[0].severity).toBe('success')

    wrapper.unmount()
  })

  it('reports a failed yank with an error toast instead of claiming success', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    mocks.SetText.mockRejectedValueOnce(new Error('no clipboard'))
    const wrapper = mount(TasksView)
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }))
    await flushPromises()

    const { toasts } = useToasts()
    expect(toasts.value).toHaveLength(1)
    expect(toasts.value[0].message).toBe('Could not copy to the clipboard')
    expect(toasts.value[0].severity).toBe('error')

    wrapper.unmount()
  })

  it('does nothing for y when nothing is selected', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    const wrapper = mount(TasksView)
    await flushPromises()
    useTasks().select(null)
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }))
    await flushPromises()
    expect(mocks.SetText).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('ignores y while a confirm dialog is stacked on top', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    const wrapper = mount(TasksView)
    await flushPromises()

    await wrapper.get('[data-testid="task-delete"]').trigger('click')
    expect(document.querySelector('[data-testid="task-delete-confirm"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }))
    await flushPromises()
    expect(mocks.SetText).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('selects the first row on the first keypress when nothing is selected', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()
    // Clear the auto-selected row directly through the singleton, isolating
    // this from the auto-select behaviour covered above.
    useTasks().select(null)
    await flushPromises()
    expect(wrapper.find('[data-testid="task-detail-title"]').exists()).toBe(false)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }))
    await flushPromises()
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')

    wrapper.unmount()
  })

  it('ignores tree navigation keys while a confirm dialog is stacked on top', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')

    await wrapper.get('[data-testid="task-delete"]').trigger('click')
    expect(document.querySelector('[data-testid="task-delete-confirm"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
    await flushPromises()
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t1')

    wrapper.unmount()
  })

  it('shows a failed direct status change inline, and clears it when the selection changes', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    mocks.SetTaskStatus.mockRejectedValueOnce(
      Object.assign(new Error('boom'), { cause: { kind: 'internal', message: 'write failed' } }),
    )
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    await chooseOption(wrapper, 'task-status-select', 'in_progress')
    await flushPromises()
    expect(wrapper.get('[data-testid="task-status-error"]').text()).toContain('write failed')

    // A failure belongs to the selection that produced it, not the next one.
    await selectRow(wrapper, 't2')
    expect(wrapper.find('[data-testid="task-status-error"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('keeps the confirm dialog open carrying the failure when a confirmed status change fails', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    mocks.SetTaskStatus.mockRejectedValueOnce(
      Object.assign(new Error('boom'), { cause: { kind: 'internal', message: 'write failed' } }),
    )
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    await chooseOption(wrapper, 'task-status-select', 'cancelled')
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="task-cancel-confirm-confirm"]')!.click()
    await flushPromises()

    expect(document.querySelector('[data-testid="task-cancel-confirm"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="task-cancel-confirm-error"]')?.textContent).toContain('write failed')

    wrapper.unmount()
  })

  it('jumps to a blocker when its chip is clicked, naming its status and the blocked cause', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1', { blocked: true }), task('t2')])
    mocks.ReadTaskDetail.mockImplementation((id: string) =>
      Promise.resolve(
        id === 't1'
          ? detailFrom(task('t1', { blocked: true }), {
              blockers: [{ id: 't2', title: 'Task t2', status: 'in_progress' }],
            })
          : detailFrom(task(id)),
      ),
    )
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    expect(wrapper.get('[data-testid="task-detail-blocked"]').text()).toBe('Blocked by 1 blocking task')
    expect(wrapper.get('[data-testid="task-blocker-status"]').text()).toBe('In Progress')

    await wrapper.get('[data-testid="task-blocker-chip"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t2')

    wrapper.unmount()
  })

  it('explains a blocked parent with no explicit blockers by its open subtasks', async () => {
    const parent = task('t1', { blocked: true })
    mocks.ListTasks.mockResolvedValue([
      parent,
      task('c1', { parentId: 't1' }),
      task('c2', { parentId: 't1', status: 'done' }),
    ])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(parent))
    const wrapper = mount(TasksView)
    await flushPromises()
    await selectRow(wrapper, 't1')

    expect(wrapper.get('[data-testid="task-detail-blocked"]').text()).toBe('Blocked by 1 open subtask')

    wrapper.unmount()
  })

  it('filters the tree by title, keeping ancestors of a match, with its own empty-state copy', async () => {
    mocks.ListTasks.mockResolvedValue([
      task('e1', { type: 'epic', title: 'Epic Alpha' }),
      task('c1', { parentId: 'e1', title: 'Fix parser' }),
      task('t2', { title: 'Write docs' }),
    ])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()
    expect(wrapper.findAll('[data-testid="task-row"]')).toHaveLength(3)

    await wrapper.get('[data-testid="tasks-search"]').setValue('parser')
    const rows = wrapper.findAll('[data-testid="task-row"]')
    expect(rows.map((row) => row.attributes('data-id'))).toEqual(['e1', 'c1'])

    await wrapper.get('[data-testid="tasks-search"]').setValue('zzz')
    expect(wrapper.get('[data-testid="tasks-empty-filter"]').text()).toBe('No tasks match this search.')

    wrapper.unmount()
  })

  it('folds and unfolds the selected node with ArrowLeft/ArrowRight and h/l, walking between parent and child', async () => {
    mocks.ListTasks.mockResolvedValue([task('e1', { type: 'epic' }), task('c1', { parentId: 'e1' })])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    const wrapper = mount(TasksView)
    await flushPromises()
    // The first row (e1) is auto-selected; both rows render while expanded.
    expect(wrapper.findAll('[data-testid="task-row"]')).toHaveLength(2)

    async function press(key: string): Promise<void> {
      window.dispatchEvent(new KeyboardEvent('keydown', { key }))
      await flushPromises()
    }

    await press('ArrowLeft')
    expect(wrapper.findAll('[data-testid="task-row"]')).toHaveLength(1)
    await press('l')
    expect(wrapper.findAll('[data-testid="task-row"]')).toHaveLength(2)
    await press('ArrowRight') // expanded: step into the first child
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task c1')
    await press('h') // leaf: walk back up to the parent
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task e1')

    wrapper.unmount()
  })

  it('names the scoped repo in the empty state, keeps it selectable, and offers show-all', async () => {
    mocks.TaskRepoKeys.mockResolvedValue(['acme/site'])
    useTasks().setRepoKey('acme/empty')
    const wrapper = mount(TasksView)
    await flushPromises()

    expect(wrapper.get('[data-testid="tasks-empty"]').text()).toContain('No tasks in acme/empty')
    expect(wrapper.get('[data-testid="tasks-repo-select"]').text()).toContain('acme/empty')

    mocks.ListTasks.mockResolvedValue([task('t1')])
    await wrapper.get('[data-testid="tasks-empty-show-all"]').trigger('click')
    await flushPromises()
    expect(useTasks().repoKey.value).toBe('')
    expect(wrapper.findAll('[data-testid="task-row"]')).toHaveLength(1)

    wrapper.unmount()
  })

  // The real app's keydowns bubble from the focused element, so this test
  // dispatches on the select's trigger (focused on open) rather than window —
  // that is the path where AppSelect's own preventDefault/stopPropagation can
  // shield the tree hotkeys and the overlay's escape-to-close.
  it('routes keys to an open select popover instead of the tree underneath it', async () => {
    mocks.ListTasks.mockResolvedValue([task('e1', { type: 'epic' }), task('c1', { parentId: 'e1' })])
    mocks.ReadTaskDetail.mockImplementation((id: string) => Promise.resolve(detailFrom(task(id))))
    // Attached, unlike the other mounts: focus() only works on elements in the
    // document, and the focused trigger is the mechanism under test.
    const wrapper = mount(TasksView, { attachTo: document.body })
    await flushPromises()
    expect(useTasks().selectedId.value).toBe('e1')

    await openSelect(wrapper, 'task-status-select')
    await flushPromises()
    const trigger = document.activeElement as HTMLElement
    expect(trigger?.getAttribute('data-testid')).toBe('task-status-select')

    function press(key: string): void {
      trigger.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
    }

    press('ArrowDown')
    await flushPromises()
    expect(useTasks().selectedId.value).toBe('e1') // the tree did not move

    press('ArrowLeft')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="task-row"]')).toHaveLength(2) // the epic did not fold

    press('j')
    await flushPromises()
    expect(useTasks().selectedId.value).toBe('e1')

    press('Escape')
    await flushPromises()
    expect(document.querySelector('[data-testid="task-status-select-popover"]')).toBeNull() // Escape closed the popover…
    expect(wrapper.emitted('close')).toBeUndefined() // …not the overlay

    wrapper.unmount()
  })

  it('lets a stacked confirm dialog take Escape first, then closes the overlay on the next Escape', async () => {
    mocks.ListTasks.mockResolvedValue([task('t1')])
    mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
    const wrapper = mount(TasksView)
    await flushPromises()

    await wrapper.get('[data-testid="task-delete"]').trigger('click')
    expect(document.querySelector('[data-testid="task-delete-confirm"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[data-testid="task-delete-confirm"]')).toBeNull()
    expect(wrapper.emitted('close')).toBeUndefined()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })

  describe('board', () => {
    beforeEach(() => {
      localStorage.setItem('hive.tasks.view', 'board')
    })

    async function dropOn(wrapper: ReturnType<typeof mount>, id: string, status: string): Promise<void> {
      await wrapper.get(`[data-testid="task-card"][data-id="${id}"]`).trigger('dragstart')
      const column = wrapper.get(`[data-testid="tasks-board-column-${status}"]`)
      await column.trigger('dragover')
      await column.trigger('drop')
      await flushPromises()
    }

    it('draws a column for every status whatever the filter, and hides the filter', async () => {
      localStorage.setItem('hive.tasks.filter', 'open')
      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2', { status: 'done' })])
      const wrapper = mount(TasksView)
      await flushPromises()

      const columns = wrapper.findAll('section[data-testid^="tasks-board-column-"]')
      expect(columns.map((column) => column.attributes('data-status'))).toEqual([
        'open',
        'in_progress',
        'done',
        'cancelled',
      ])
      expect(wrapper.find('[data-testid="tasks-board-column-done"] [data-testid="task-card"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="tasks-filter"]').exists()).toBe(false)
    })

    it('moves a card into a column that is empty', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      const wrapper = mount(TasksView)
      await flushPromises()

      await dropOn(wrapper, 't1', 'done')

      expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'done')
    })

    it('moves a card to the column it is dropped on', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      const wrapper = mount(TasksView)
      await flushPromises()

      await dropOn(wrapper, 't1', 'in_progress')

      expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'in_progress')
    })

    it('ignores a drop on the column the card is already in', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      const wrapper = mount(TasksView)
      await flushPromises()

      await dropOn(wrapper, 't1', 'open')

      expect(mocks.SetTaskStatus).not.toHaveBeenCalled()
    })

    it('confirms a drop on Cancelled before applying it', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      const wrapper = mount(TasksView, { attachTo: document.body })
      await flushPromises()

      await dropOn(wrapper, 't1', 'cancelled')
      expect(mocks.SetTaskStatus).not.toHaveBeenCalled()

      document.querySelector<HTMLElement>('[data-testid="task-cancel-confirm-confirm"]')?.click()
      await flushPromises()
      expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'cancelled')

      wrapper.unmount()
    })

    function drawer(): Element | null {
      return document.querySelector('[data-testid="task-detail-pane"]')
    }

    it('opens the detail in a drawer over the board, and closes it on Escape before the overlay', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
      const wrapper = mount(TasksView)
      await flushPromises()
      expect(drawer()).toBeNull()

      await wrapper.get('[data-testid="task-card"][data-id="t1"]').trigger('click')
      await flushPromises()
      expect(drawer()?.querySelector('[data-testid="task-detail-title"]')?.textContent?.trim()).toBe('Task t1')
      expect(wrapper.find('[data-testid="task-detail-pane"]').exists()).toBe(false)

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await flushPromises()
      expect(drawer()).toBeNull()
      expect(wrapper.emitted('close')).toBeUndefined()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await flushPromises()
      expect(wrapper.emitted('close')).toHaveLength(1)

      wrapper.unmount()
    })

    it('closes the drawer from its close button and its backdrop', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
      const wrapper = mount(TasksView)
      await flushPromises()

      await wrapper.get('[data-testid="task-card"][data-id="t1"]').trigger('click')
      await flushPromises()
      document.querySelector<HTMLElement>('[data-testid="task-detail-close"]')!.click()
      await flushPromises()
      expect(drawer()).toBeNull()

      await wrapper.get('[data-testid="task-card"][data-id="t1"]').trigger('click')
      await flushPromises()
      document.querySelector<HTMLElement>('[data-testid="task-detail-pane-backdrop"]')!.click()
      await flushPromises()
      expect(drawer()).toBeNull()

      wrapper.unmount()
    })

    it('stops the board keys while the drawer is open', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
      mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
      const wrapper = mount(TasksView)
      await flushPromises()

      await wrapper.get('[data-testid="task-card"][data-id="t1"]').trigger('click')
      await flushPromises()
      const { selectedId } = useTasks()
      const before = selectedId.value

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'd' }))
      await flushPromises()
      expect(selectedId.value).toBe(before)
      expect(mocks.SetTaskStatus).not.toHaveBeenCalled()

      wrapper.unmount()
    })

    it('closes the drawer when its task is deleted, rather than showing the next one', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
      mocks.ReadTaskDetail.mockResolvedValue(detailFrom(task('t1')))
      const wrapper = mount(TasksView)
      await flushPromises()

      await wrapper.get('[data-testid="task-card"][data-id="t1"]').trigger('click')
      await flushPromises()
      document.querySelector<HTMLElement>('[data-testid="task-delete"]')!.click()
      await flushPromises()
      mocks.ListTasks.mockResolvedValue([task('t2')])
      document.querySelector<HTMLElement>('[data-testid="task-delete-confirm-confirm"]')!.click()
      await flushPromises()

      expect(mocks.DeleteTask).toHaveBeenCalledWith('t1')
      expect(drawer()).toBeNull()

      wrapper.unmount()
    })

    it('steps between columns with h/l, keeping the row position where the column allows', async () => {
      mocks.ListTasks.mockResolvedValue([
        task('o1', { updatedAt: '2026-01-03T00:00:00Z' }),
        task('o2', { updatedAt: '2026-01-02T00:00:00Z' }),
        task('p1', { status: 'in_progress', updatedAt: '2026-01-03T00:00:00Z' }),
      ])
      const wrapper = mount(TasksView)
      await flushPromises()
      const selected = () => wrapper.find('[data-testid="task-card"][aria-selected="true"]').attributes('data-id')
      expect(selected()).toBe('o1')

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
      await flushPromises()
      expect(selected()).toBe('o2')

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'l' }))
      await flushPromises()
      expect(selected()).toBe('p1')

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'h' }))
      await flushPromises()
      expect(selected()).toBe('o1')

      wrapper.unmount()
    })
  })

  describe('row actions', () => {
    function menuEntry(testid: string): HTMLElement {
      return document.querySelector<HTMLElement>(`[data-testid="${testid}"]`)!
    }

    function row(wrapper: ReturnType<typeof mount>, id: string) {
      return wrapper.get(`[data-testid="task-row"][data-id="${id}"]`)
    }

    it('marks a task done from its row without selecting it', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
      const wrapper = mount(TasksView)
      await flushPromises()
      const { selectedId } = useTasks()
      expect(selectedId.value).toBe('t1')

      await row(wrapper, 't2').get('[data-testid="task-row-done"]').trigger('click')
      await flushPromises()

      expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t2', 'done')
      expect(selectedId.value).toBe('t1')
      wrapper.unmount()
    })

    it('reopens a closed task from its row', async () => {
      localStorage.setItem('hive.tasks.filter', 'all')
      mocks.ListTasks.mockResolvedValue([task('t1', { status: 'done' })])
      const wrapper = mount(TasksView)
      await flushPromises()

      await row(wrapper, 't1').get('[data-testid="task-row-done"]').trigger('click')
      await flushPromises()

      expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'open')
      wrapper.unmount()
    })

    it('confirms a cascade when an epic is marked done from its row', async () => {
      mocks.ListTasks.mockResolvedValue([task('e1', { type: 'epic' }), task('t1', { parentId: 'e1' })])
      const wrapper = mount(TasksView)
      await flushPromises()

      await row(wrapper, 'e1').get('[data-testid="task-row-done"]').trigger('click')
      await flushPromises()

      expect(mocks.SetTaskStatus).not.toHaveBeenCalled()
      expect(document.querySelector('[data-testid="task-cascade-confirm"]')).not.toBeNull()
      wrapper.unmount()
    })

    it('sets any status from the menu on the status icon', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      const wrapper = mount(TasksView)
      await flushPromises()

      await row(wrapper, 't1').get('[data-testid="task-row-status"]').trigger('click')
      expect(document.querySelector('[data-testid="task-row-menu-delete"]')).toBeNull()
      menuEntry('task-row-menu-status-in_progress').click()
      await flushPromises()

      expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t1', 'in_progress')
      expect(document.querySelector('[data-testid="task-row-menu"]')).toBeNull()
      wrapper.unmount()
    })

    it('does nothing when the menu picks the status the task already has', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      const wrapper = mount(TasksView)
      await flushPromises()

      await row(wrapper, 't1').get('[data-testid="task-row-status"]').trigger('click')
      menuEntry('task-row-menu-status-open').click()
      await flushPromises()

      expect(mocks.SetTaskStatus).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    it('deletes a task from the context menu after confirming', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
      const wrapper = mount(TasksView)
      await flushPromises()

      await row(wrapper, 't2').trigger('contextmenu')
      menuEntry('task-row-menu-delete').click()
      await flushPromises()
      expect(mocks.DeleteTask).not.toHaveBeenCalled()

      menuEntry('task-delete-confirm-confirm').click()
      await flushPromises()
      expect(mocks.DeleteTask).toHaveBeenCalledWith('t2')
      wrapper.unmount()
    })

    it('copies a task id from the more menu', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1')])
      const wrapper = mount(TasksView)
      await flushPromises()

      await row(wrapper, 't1').get('[data-testid="task-row-more"]').trigger('click')
      menuEntry('task-row-menu-copy').click()
      await flushPromises()

      expect(mocks.SetText).toHaveBeenCalledWith('t1')
      wrapper.unmount()
    })

    it('keeps one context menu open when a second row is right-clicked', async () => {
      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2')])
      const wrapper = mount(TasksView, { attachTo: document.body })
      await flushPromises()

      await row(wrapper, 't1').trigger('contextmenu')
      await row(wrapper, 't2').trigger('contextmenu')
      await flushPromises()

      expect(document.querySelectorAll('[data-testid="task-row-menu"]')).toHaveLength(1)
      wrapper.unmount()
    })

    it('marks the selected task done with d, and moves to the row that takes its place', async () => {
      localStorage.setItem('hive.tasks.filter', 'open')
      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2'), task('t3')])
      const wrapper = mount(TasksView)
      await flushPromises()
      await selectRow(wrapper, 't2')

      mocks.ListTasks.mockResolvedValue([task('t1'), task('t2', { status: 'done' }), task('t3')])
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'd' }))
      await flushPromises()

      expect(mocks.SetTaskStatus).toHaveBeenCalledWith('t2', 'done')
      expect(useTasks().selectedId.value).toBe('t3')
      wrapper.unmount()
    })
  })

  it('lists an epic sub-tasks in the detail pane and opens one on click', async () => {
    const epic = task('e1', { type: 'epic' })
    mocks.ListTasks.mockResolvedValue([
      epic,
      task('t1', { parentId: 'e1', status: 'done' }),
      task('t2', { parentId: 'e1' }),
    ])
    mocks.ReadTaskDetail.mockImplementation((id: string) =>
      Promise.resolve(detailFrom(id === 'e1' ? epic : task(id, { parentId: 'e1' }))),
    )
    const wrapper = mount(TasksView)
    await flushPromises()

    const section = wrapper.get('[data-testid="task-subtasks"]')
    expect(section.text()).toContain('1/2')
    expect(section.findAll('[data-testid="task-subtask"]').map((row) => row.attributes('data-id'))).toEqual([
      't1',
      't2',
    ])

    await section.get('[data-testid="task-subtask"][data-id="t2"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="task-detail-title"]').text()).toBe('Task t2')
    expect(wrapper.get('[data-testid="task-detail-parent"]').text()).toBe('Task e1')
  })

  it('makes the repository picker searchable', async () => {
    mocks.TaskRepoKeys.mockResolvedValue(['acme/site', 'acme/api'])
    const wrapper = mount(TasksView, { attachTo: document.body })
    await flushPromises()

    await openSelect(wrapper, 'tasks-repo-select')
    expect(document.querySelector('[data-testid="tasks-repo-select-search"]')).not.toBeNull()

    wrapper.unmount()
  })
})
