import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  ListTasks: vi.fn(),
  ReadTaskDetail: vi.fn(),
  SetTaskStatus: vi.fn(),
  DeleteTask: vi.fn(),
  PruneTasks: vi.fn(),
  TaskRepoKeys: vi.fn(),
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

import { h } from 'vue'
import HubOverlay from '../HubOverlay.vue'
import TasksView from '../../TasksView.vue'

function el<T extends HTMLElement>(testid: string): T {
  const element = document.querySelector<T>(`[data-testid="${testid}"]`)
  if (!element) throw new Error(`Missing ${testid}`)
  return element
}

// TasksView is the child here because it owns Escape and the j/k walk the
// overlay's focus handling exists for.
function mountTasks(onViewClose = vi.fn()) {
  return mount(HubOverlay, {
    props: { label: 'Tasks', testid: 'tasks-overlay' },
    slots: { default: () => h(TasksView, { onClose: onViewClose }) },
  })
}

describe('HubOverlay', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.ListTasks.mockResolvedValue([])
    mocks.TaskRepoKeys.mockResolvedValue([])
  })

  it('hosts its view inside a large teleported panel', async () => {
    const wrapper = mountTasks()
    await flushPromises()

    expect(el('tasks-overlay')).toBeTruthy()
    expect(el('tasks-view')).toBeTruthy()

    wrapper.unmount()
  })

  it('emits close when the backdrop is clicked', async () => {
    const wrapper = mountTasks()
    await flushPromises()

    el('tasks-overlay-backdrop').click()
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })

  it('does not close when a click lands inside the panel', async () => {
    const wrapper = mountTasks()
    await flushPromises()

    el('tasks-overlay').click()
    expect(wrapper.emitted('close')).toBeUndefined()

    wrapper.unmount()
  })

  // The regression: opened over a terminal, the pane's textarea kept focus, so
  // TasksView's j/k walk read every key as typing into an editable target and
  // the letters went to the shell instead.
  it('takes focus off the editable element it opened over, and hands it back on close', async () => {
    const textarea = document.createElement('textarea')
    document.body.append(textarea)
    textarea.focus()

    const wrapper = mountTasks()
    await flushPromises()
    expect(document.activeElement).toBe(el('tasks-overlay'))

    wrapper.unmount()
    await flushPromises()
    expect(document.activeElement).toBe(textarea)

    textarea.remove()
  })

  it('leaves Escape to the view it hosts', async () => {
    const onViewClose = vi.fn()
    const wrapper = mountTasks(onViewClose)
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(onViewClose).toHaveBeenCalledOnce()
    expect(wrapper.emitted('close')).toBeUndefined()

    wrapper.unmount()
  })
})
