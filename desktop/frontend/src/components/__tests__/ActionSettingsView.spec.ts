import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ActionSettingsView from '../ActionSettingsView.vue'
import type { EditableAction } from '../../composables/useActionsSettings'

const mocks = vi.hoisted(() => ({ ListActions: vi.fn(), CreateAction: vi.fn(), UpdateAction: vi.fn(), DeleteAction: vi.fn(), On: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/desktop/actionsservice', () => ({
  ListActions: mocks.ListActions, CreateAction: mocks.CreateAction, UpdateAction: mocks.UpdateAction, DeleteAction: mocks.DeleteAction,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

const launch: EditableAction = {
  id: 'review', label: 'Review', type: 'launch-session', showInDetail: true, appliesTo: ['pr'],
  launch: { promptTemplate: 'Review {{ .Payload }}', repoTemplate: 'https://repo', agent: 'codex' },
}

function mountSettings(actions: EditableAction[] = [launch]) {
  mocks.ListActions.mockResolvedValue({ actions, error: '' })
  mocks.On.mockReturnValue(() => {})
  return mount(ActionSettingsView)
}

beforeEach(() => { vi.clearAllMocks() })

describe('ActionSettingsView', () => {
  it('round-trips every editable launch field', async () => {
    mocks.UpdateAction.mockResolvedValue(launch)
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-testid="action-row-review"] button').trigger('click')
    await wrapper.get('[data-testid="action-launch-agent"]').setValue('claude')
    await wrapper.get('[data-testid="action-launch-repo"]').setValue('https://other')
    await wrapper.get('[data-testid="action-launch-prompt"]').setValue('Updated')
    await wrapper.get('[data-testid="action-save"]').trigger('click')

    expect(mocks.UpdateAction).toHaveBeenCalledWith('review', expect.objectContaining({
      id: 'review',
      launch: { promptTemplate: 'Updated', repoTemplate: 'https://other', agent: 'claude' },
    }))
    const sent = mocks.UpdateAction.mock.calls[0][1]
    expect(sent.shell).toBeUndefined()
    expect(sent.message).toBeUndefined()
  })

  it('creates exactly one shell branch with all shell fields and deterministic env input', async () => {
    mocks.CreateAction.mockResolvedValue({ id: 'run', label: 'Run', type: 'shell' })
    const wrapper = mountSettings([])
    await flushPromises()
    await wrapper.get('[data-testid="action-create"]').trigger('click')
    expect(wrapper.find('[data-testid="action-auto-apply"]').exists()).toBe(false)
    await wrapper.get('[data-testid="action-id"]').setValue('run')
    await wrapper.get('[data-testid="action-label"]').setValue('Run')
    await wrapper.get('[data-testid="action-type"]').setValue('shell')
    await wrapper.get('[data-testid="action-shell-command"]').setValue('go test ./...')
    await wrapper.get('[data-testid="action-shell-cwd"]').setValue('/repo')
    await wrapper.get('[data-testid="action-shell-timeout"]').setValue('30s')
    await wrapper.get('[data-testid="action-shell-env"]').setValue('ZED=z\nALPHA=a')
    await wrapper.get('[data-testid="action-save"]').trigger('click')

    expect(mocks.CreateAction).toHaveBeenCalledWith(expect.objectContaining({
      id: 'run', type: 'shell', launch: undefined, message: undefined,
      shell: { commandTemplate: 'go test ./...', cwd: '/repo', timeout: '30s', env: { ZED: 'z', ALPHA: 'a' } },
    }))
  })

  it('keeps publish-message terminology honest and surfaces deletion errors', async () => {
    mocks.DeleteAction.mockRejectedValue(new Error('flow-a blocks deletion'))
    const wrapper = mountSettings([{ id: 'event', label: 'Event', type: 'publish-message', showInDetail: false, appliesTo: [], message: { topic: 'updates', messageTemplate: 'hello' } }])
    await flushPromises()
    await wrapper.get('[data-testid="action-row-event"] button:last-child').trigger('click')
    await wrapper.get('[data-testid="action-delete-confirm"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="actions-error"]').text()).toContain('flow-a blocks deletion')

    await wrapper.get('[data-testid="action-row-event"] button').trigger('click')
    expect(wrapper.get('[data-testid="action-type"]').text()).toContain('Publish message')
    expect(wrapper.find('[data-testid="action-message-topic"]').exists()).toBe(true)
  })
})
