import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LauncherSettingsView from '../LauncherSettingsView.vue'
import type { Launcher } from '../../composables/useActionsSettings'

const mocks = vi.hoisted(() => ({
  ListActions: vi.fn(),
  CreateAction: vi.fn(), UpdateAction: vi.fn(), DeleteAction: vi.fn(), ReorderActions: vi.fn(),
  CreateLauncher: vi.fn(), UpdateLauncher: vi.fn(), DeleteLauncher: vi.fn(),
  KeybindingSettings: vi.fn(), SetKeybindingSettings: vi.fn(),
  On: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/actionsservice', () => ({
  ListActions: mocks.ListActions,
  CreateAction: mocks.CreateAction, UpdateAction: mocks.UpdateAction,
  DeleteAction: mocks.DeleteAction, ReorderActions: mocks.ReorderActions,
  CreateLauncher: mocks.CreateLauncher, UpdateLauncher: mocks.UpdateLauncher, DeleteLauncher: mocks.DeleteLauncher,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  KeybindingSettings: mocks.KeybindingSettings,
  SetKeybindingSettings: mocks.SetKeybindingSettings,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

const lazygit: Launcher = { id: 'lazygit', label: 'lazygit', command: 'lazygit', cwd: '', icon: 'git-branch' }

function mountSettings(launchers: Launcher[] = [lazygit]) {
  mocks.ListActions.mockResolvedValue({ actions: [], launchers, error: '' })
  mocks.On.mockReturnValue(() => {})
  return mount(LauncherSettingsView, { attachTo: document.body })
}
function el<T extends HTMLElement>(id: string): T { return document.querySelector<T>(`[data-testid="${id}"]`)! }
async function setValue(element: HTMLInputElement, value: string): Promise<void> {
  element.value = value
  element.dispatchEvent(new Event('input', { bubbles: true }))
  element.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
  mocks.KeybindingSettings.mockResolvedValue({ overrides: {} })
  mocks.SetKeybindingSettings.mockResolvedValue(undefined)
})

describe('LauncherSettingsView', () => {
  it('lists the launchers from the same catalog read as the actions', async () => {
    const wrapper = mountSettings()
    await flushPromises()

    expect(wrapper.find('[data-testid="launcher-row-lazygit"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="launchers-source"]').text()).toContain('1 quick terminal')
    wrapper.unmount()
  })

  it('creates a launcher with only the fields a launcher has', async () => {
    mocks.CreateLauncher.mockResolvedValue({ id: 'btop', label: 'btop', command: 'btop' })
    const wrapper = mountSettings([])
    await flushPromises()

    await wrapper.get('[data-testid="launcher-create"]').trigger('click')
    await setValue(el<HTMLInputElement>('launcher-id'), 'btop')
    await setValue(el<HTMLInputElement>('launcher-label'), 'btop')
    await setValue(el<HTMLInputElement>('launcher-command'), 'btop')
    el<HTMLButtonElement>('launcher-save').click()
    await flushPromises()

    expect(mocks.CreateLauncher).toHaveBeenCalledWith({ id: 'btop', label: 'btop', command: 'btop', cwd: '', icon: '' })
    wrapper.unmount()
  })

  it('refuses to save without a command — a launcher with none is an unnamed shell', async () => {
    const wrapper = mountSettings([])
    await flushPromises()

    await wrapper.get('[data-testid="launcher-create"]').trigger('click')
    await setValue(el<HTMLInputElement>('launcher-id'), 'btop')
    await setValue(el<HTMLInputElement>('launcher-label'), 'btop')
    el<HTMLButtonElement>('launcher-save').click()
    await flushPromises()

    expect(mocks.CreateLauncher).not.toHaveBeenCalled()
    expect(el('launcher-editor-error').textContent).toContain('command are required')
    wrapper.unmount()
  })

  // The binding lives in settings.yaml under launcher.<id>, so the row says
  // whether there is one rather than offering a second place to set it.
  it('reports whether a launcher is bound', async () => {
    mocks.KeybindingSettings.mockResolvedValue({ overrides: { 'launcher.lazygit': ['alt+g'] } })
    const { initializeKeybindings } = await import('../../composables/useKeybindings')
    const { setLauncherCommands } = await import('../../keybindings/catalog')
    setLauncherCommands([{ id: 'launcher.lazygit', title: 'lazygit', group: 'Quick terminals', defaultCombos: [], context: 'global' }])
    initializeKeybindings()

    const wrapper = mountSettings()
    await vi.waitFor(() => expect(wrapper.get('[data-testid="launcher-shortcut-lazygit"]').text()).not.toContain('Unbound'))
    setLauncherCommands([])
    wrapper.unmount()
  })

  it('deletes a launcher through the confirmation dialog', async () => {
    mocks.DeleteLauncher.mockResolvedValue(undefined)
    const wrapper = mountSettings()
    await flushPromises()

    await wrapper.get('[data-testid="launcher-row-lazygit"] button[aria-label="Delete"]').trigger('click')
    await flushPromises()
    el<HTMLButtonElement>('confirmation-dialog-confirm').click()
    await flushPromises()

    expect(mocks.DeleteLauncher).toHaveBeenCalledWith('lazygit')
    wrapper.unmount()
  })
})
