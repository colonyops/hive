import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ActionSettingsView from '../ActionSettingsView.vue'
import type { EditableAction } from '../../composables/useActionsSettings'

const mocks = vi.hoisted(() => ({
  ListActions: vi.fn(),
  CreateAction: vi.fn(),
  UpdateAction: vi.fn(),
  DeleteAction: vi.fn(),
  ReorderActions: vi.fn(),
  SessionLaunchWorkspaces: vi.fn(),
  On: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/actionsservice', () => ({
  ListActions: mocks.ListActions,
  CreateAction: mocks.CreateAction,
  UpdateAction: mocks.UpdateAction,
  DeleteAction: mocks.DeleteAction,
  ReorderActions: mocks.ReorderActions,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionLaunchWorkspaces: mocks.SessionLaunchWorkspaces,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

const launch: EditableAction = {
  id: 'review',
  label: 'Review',
  type: 'launch-session',
  showInDetail: true,
  targets: ['item'],
  appliesTo: ['pr'],
  launch: { promptTemplate: 'Review {{ .Payload }}', repoTemplate: 'https://repo', agent: 'codex' },
}
function mountSettings(actions: EditableAction[] = [launch]) {
  mocks.ListActions.mockResolvedValue({ actions, error: '' })
  mocks.On.mockReturnValue(() => {})
  return mount(ActionSettingsView, { attachTo: document.body })
}
function editor<T extends HTMLElement>(id: string): T {
  return document.querySelector<T>(`[data-testid="${id}"]`)!
}
function shell(id: string): EditableAction {
  return {
    id,
    label: id.toUpperCase(),
    type: 'shell',
    showInDetail: true,
    targets: ['item'],
    appliesTo: [],
    shell: { commandTemplate: 'true' },
  }
}
function rowIds(wrapper: VueWrapper): string[] {
  return wrapper
    .findAll('[data-testid^="action-row-"]')
    .map((row) => row.attributes('data-testid')!.replace('action-row-', ''))
}
async function setValue(
  element: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement,
  value: string,
): Promise<void> {
  element.value = value
  element.dispatchEvent(new Event('input', { bubbles: true }))
  element.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.SessionLaunchWorkspaces.mockResolvedValue([{ dir: 'alerts', name: 'Alert triage', supportsPrompt: true }])
  document.body.innerHTML = ''
})

describe('ActionSettingsView', () => {
  it('opens create/edit in a right slideover and round-trips launch fields', async () => {
    mocks.UpdateAction.mockResolvedValue(launch)
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-testid="action-row-review"] button').trigger('click')
    expect(editor('action-editor').className).toContain('right-0')
    expect(editor('action-id') instanceof HTMLInputElement && editor<HTMLInputElement>('action-id').disabled).toBe(true)
    await setValue(editor<HTMLInputElement>('action-launch-agent'), 'claude')
    await setValue(editor<HTMLInputElement>('action-launch-repo'), 'https://other')
    await setValue(editor<HTMLTextAreaElement>('action-launch-prompt'), 'Updated')
    editor<HTMLButtonElement>('action-save').click()
    await flushPromises()
    expect(mocks.UpdateAction).toHaveBeenCalledWith(
      'review',
      expect.objectContaining({
        id: 'review',
        launch: { promptTemplate: 'Updated', repoTemplate: 'https://other', agent: 'claude' },
      }),
    )
    expect(mocks.UpdateAction.mock.calls[0][1].shell).toBeUndefined()
    wrapper.unmount()
  })

  it('saves a workspace target and clears repository-only fields', async () => {
    mocks.UpdateAction.mockResolvedValue(launch)
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-testid="action-row-review"] button').trigger('click')
    await editor<HTMLButtonElement>('action-launch-target').click()
    await flushPromises()
    await editor<HTMLButtonElement>('action-launch-target-option-workspace').click()
    await flushPromises()
    await editor<HTMLButtonElement>('action-launch-workspace').click()
    await flushPromises()
    await editor<HTMLButtonElement>('action-launch-workspace-option-alerts').click()
    await flushPromises()
    editor<HTMLButtonElement>('action-save').click()
    await flushPromises()
    expect(mocks.UpdateAction.mock.calls[0][1].launch).toEqual(
      expect.objectContaining({
        workspace: 'alerts',
        repoTemplate: '',
        agent: '',
        postHook: '',
        postHookTimeout: '',
      }),
    )
    wrapper.unmount()
  })

  it('saves a launch post hook and its timeout', async () => {
    mocks.UpdateAction.mockResolvedValue(launch)
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-testid="action-row-review"] button').trigger('click')
    await setValue(editor<HTMLTextAreaElement>('action-launch-post-hook'), 'gh pr checkout {{ .Payload.num }} && zed .')
    await setValue(editor<HTMLInputElement>('action-launch-post-hook-timeout'), '2m')
    editor<HTMLButtonElement>('action-save').click()
    await flushPromises()
    expect(mocks.UpdateAction.mock.calls[0][1].launch).toEqual(
      expect.objectContaining({ postHook: 'gh pr checkout {{ .Payload.num }} && zed .', postHookTimeout: '2m' }),
    )
    wrapper.unmount()
  })

  it('saves shell fields and closes the slideover via Escape', async () => {
    mocks.CreateAction.mockResolvedValue({ id: 'run', label: 'Run', type: 'shell' })
    const wrapper = mountSettings([])
    await flushPromises()
    await wrapper.get('[data-testid="action-create"]').trigger('click')
    await setValue(editor<HTMLInputElement>('action-id'), 'run')
    await setValue(editor<HTMLInputElement>('action-label'), 'Run')
    editor<HTMLButtonElement>('action-type').click()
    await flushPromises()
    editor<HTMLButtonElement>('action-type-option-shell').click()
    await flushPromises()
    await setValue(editor<HTMLTextAreaElement>('action-shell-command'), 'go test ./...')
    await setValue(editor<HTMLInputElement>('action-shell-cwd'), '/repo')
    await setValue(editor<HTMLInputElement>('action-shell-timeout'), '30s')
    await setValue(editor<HTMLTextAreaElement>('action-shell-env'), 'ZED=z\nALPHA=a')
    editor<HTMLButtonElement>('action-save').click()
    await flushPromises()
    expect(mocks.CreateAction).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'run',
        type: 'shell',
        launch: undefined,
        message: undefined,
        shell: { commandTemplate: 'go test ./...', cwd: '/repo', timeout: '30s', env: { ZED: 'z', ALPHA: 'a' } },
      }),
    )
    await wrapper.get('[data-testid="action-create"]').trigger('click')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[data-testid="action-editor"]')).toBeNull()
    wrapper.unmount()
  })

  it('declares which surfaces offer an action, and hides the item-only fields when none do', async () => {
    mocks.CreateAction.mockResolvedValue({ id: 'zed', label: 'Zed', type: 'shell' })
    const wrapper = mountSettings([])
    await flushPromises()
    await wrapper.get('[data-testid="action-create"]').trigger('click')
    await setValue(editor<HTMLInputElement>('action-id'), 'zed')
    await setValue(editor<HTMLInputElement>('action-label'), 'Zed')
    editor<HTMLButtonElement>('action-type').click()
    await flushPromises()
    editor<HTMLButtonElement>('action-type-option-shell').click()
    await flushPromises()
    await setValue(editor<HTMLTextAreaElement>('action-shell-command'), 'zed .')

    editor<HTMLInputElement>('action-target-session').click()
    await flushPromises()
    editor<HTMLInputElement>('action-target-item').click()
    await flushPromises()
    // applies_to and the detail-pane toggle refine the item surface only, so
    // they go with it.
    expect(document.querySelector('[data-testid="action-show-in-detail"]')).toBeNull()

    editor<HTMLButtonElement>('action-save').click()
    await flushPromises()
    expect(mocks.CreateAction).toHaveBeenCalledWith(expect.objectContaining({ id: 'zed', targets: ['session'] }))
    wrapper.unmount()
  })

  it('refuses terminal targets for a launch-session action, which creates a session of its own', async () => {
    const wrapper = mountSettings([])
    await flushPromises()
    await wrapper.get('[data-testid="action-create"]').trigger('click')
    expect(editor<HTMLInputElement>('action-target-session').disabled).toBe(true)
    expect(editor<HTMLInputElement>('action-target-window').disabled).toBe(true)
    wrapper.unmount()
  })

  it('focuses the ID for new actions, the label for existing actions, traps Tab, and restores the trigger on close', async () => {
    const wrapper = mountSettings()
    await flushPromises()
    const editButton = wrapper.get('[data-testid="action-row-review"] button')
    const editButtonElement = editButton.element as HTMLButtonElement
    editButtonElement.focus()
    await editButton.trigger('click')
    await flushPromises()
    expect(document.activeElement).toBe(editor('action-label'))
    editor<HTMLButtonElement>('action-save').focus()
    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    editor<HTMLButtonElement>('action-save').dispatchEvent(tab)
    expect(tab.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(editor('resize-handle-action-editor'))
    const shiftTab = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true })
    editor('resize-handle-action-editor').dispatchEvent(shiftTab)
    expect(shiftTab.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(editor('action-save'))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.activeElement).toBe(editButtonElement)

    const createButton = wrapper.get('[data-testid="action-create"]')
    await createButton.trigger('click')
    await flushPromises()
    expect(document.activeElement).toBe(editor('action-id'))
    wrapper.unmount()
  })

  it('keeps Escape disabled while an action is saving', async () => {
    let resolveCreate!: (action: EditableAction) => void
    mocks.CreateAction.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveCreate = resolve as typeof resolveCreate
        }),
    )
    const wrapper = mountSettings([])
    await flushPromises()
    await wrapper.get('[data-testid="action-create"]').trigger('click')
    await flushPromises()
    await setValue(editor<HTMLInputElement>('action-id'), 'run')
    await setValue(editor<HTMLInputElement>('action-label'), 'Run')
    editor<HTMLButtonElement>('action-save').click()
    await flushPromises()
    expect(editor<HTMLButtonElement>('action-save').disabled).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[data-testid="action-editor"]')).not.toBeNull()
    resolveCreate({
      id: 'run',
      label: 'Run',
      type: 'launch-session',
      showInDetail: true,
      targets: ['item'],
      appliesTo: [],
    })
    await flushPromises()
    wrapper.unmount()
  })

  it('persists a dragged order and renders it without waiting for the reload', async () => {
    // jsdom rects are all zeroes, so a dragover with no clientY lands on the
    // row's bottom half — the "after" edge.
    let stored = ['a', 'b', 'c'].map(shell)
    mocks.ListActions.mockImplementation(async () => ({ actions: stored, error: '' }))
    mocks.ReorderActions.mockImplementation(async (ids: string[]) => {
      stored = ids.map((id) => stored.find((action) => action.id === id)!)
    })
    mocks.On.mockReturnValue(() => {})
    const wrapper = mount(ActionSettingsView, { attachTo: document.body })
    await flushPromises()

    await wrapper.get('[data-testid="action-row-a"]').trigger('dragstart')
    await wrapper.get('[data-testid="action-row-c"]').trigger('dragover')
    await wrapper.get('[data-testid="action-row-c"]').trigger('drop')
    expect(mocks.ReorderActions).toHaveBeenCalledWith(['b', 'c', 'a'])
    expect(rowIds(wrapper)).toEqual(['b', 'c', 'a'])
    await flushPromises()
    expect(rowIds(wrapper)).toEqual(['b', 'c', 'a'])

    // Dropping an action back where it already sits writes nothing.
    await wrapper.get('[data-testid="action-row-b"]').trigger('dragstart')
    await wrapper.get('[data-testid="action-row-c"]').trigger('dragover', { clientY: -1 })
    await wrapper.get('[data-testid="action-row-c"]').trigger('drop')
    await flushPromises()
    expect(mocks.ReorderActions).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('restores the previous order when the backend rejects the reorder', async () => {
    mocks.ReorderActions.mockRejectedValue(new Error('the catalog changed'))
    const wrapper = mountSettings(['a', 'b'].map(shell))
    await flushPromises()
    await wrapper.get('[data-testid="action-row-b"]').trigger('dragstart')
    await wrapper.get('[data-testid="action-row-a"]').trigger('dragover', { clientY: -1 })
    await wrapper.get('[data-testid="action-row-a"]').trigger('drop')
    await flushPromises()
    expect(rowIds(wrapper)).toEqual(['a', 'b'])
    expect(wrapper.get('[data-testid="actions-error"]').text()).toContain('the catalog changed')
    wrapper.unmount()
  })

  it('keeps deletion confirmation open with backend errors and allows cancel', async () => {
    mocks.DeleteAction.mockRejectedValue(new Error('flow-a blocks deletion'))
    const wrapper = mountSettings([
      {
        id: 'event',
        label: 'Event',
        type: 'publish-message',
        showInDetail: false,
        targets: ['item'],
        appliesTo: [],
        message: { topic: 'updates', messageTemplate: 'hello' },
      },
    ])
    await flushPromises()
    await wrapper.get('[data-testid="action-row-event"] button:last-child').trigger('click')
    editor<HTMLButtonElement>('confirmation-dialog-confirm').click()
    await flushPromises()
    expect(editor('confirmation-dialog-error').textContent).toContain('flow-a blocks deletion')
    expect(document.querySelector('[data-testid="confirmation-dialog"]')).not.toBeNull()
    editor<HTMLButtonElement>('confirmation-dialog-cancel').click()
    await flushPromises()
    expect(document.querySelector('[data-testid="confirmation-dialog"]')).toBeNull()
    wrapper.unmount()
  })
})
