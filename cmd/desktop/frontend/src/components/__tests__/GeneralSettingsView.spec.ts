import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import GeneralSettingsView from '../GeneralSettingsView.vue'

const mocks = vi.hoisted(() => ({
  EditorSettings: vi.fn(),
  SetEditor: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  EditorSettings: mocks.EditorSettings,
  SetEditor: mocks.SetEditor,
}))

function editorSettings(overrides: Record<string, unknown> = {}) {
  return {
    command: '',
    choices: [
      { command: 'zed', title: 'Zed', found: true },
      { command: 'code', title: 'VS Code', found: false },
    ],
    ...overrides,
  }
}

// The control persists on a debounce, so every save assertion has to run the
// clock forward first.
async function settle(): Promise<void> {
  await vi.runAllTimersAsync()
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  mocks.EditorSettings.mockResolvedValue(editorSettings())
  mocks.SetEditor.mockResolvedValue(undefined)
  document.body.innerHTML = ''
})

afterEach(() => {
  vi.useRealTimers()
})

describe('GeneralSettingsView', () => {
  it('renders the persisted editor command', async () => {
    mocks.EditorSettings.mockResolvedValue(editorSettings({ command: 'nvim' }))
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="general-editor-command"]').attributes('value')).toBe('nvim')
  })

  it('offers the detected editors and says which are not on PATH', async () => {
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="general-editor-command"]').trigger('focus')
    await flushPromises()

    const labels = Array.from(document.querySelectorAll('[role="option"]')).map((el) => el.textContent?.trim())
    expect(labels).toEqual(['None', 'Zed', 'VS Code (not found)'])
    wrapper.unmount()
  })

  it('persists a detected editor picked from the list', async () => {
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="general-editor-command"]').trigger('focus')
    await flushPromises()
    const option = document.querySelectorAll('[role="option"] button')[1] as HTMLElement
    option.click()
    await settle()

    expect(mocks.SetEditor).toHaveBeenCalledWith('zed')
    wrapper.unmount()
  })

  it('clears the setting from the None option', async () => {
    mocks.EditorSettings.mockResolvedValue(editorSettings({ command: 'zed' }))
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="general-editor-command"]').trigger('focus')
    await flushPromises()
    ;(document.querySelectorAll('[role="option"] button')[0] as HTMLElement).click()
    await settle()

    expect(mocks.SetEditor).toHaveBeenCalledWith('')
    wrapper.unmount()
  })

  // #226: the picker used to offer only the four detected commands, so anything
  // else had to be hand-written into settings.yaml.
  it('persists a command that is not in the detected catalogue', async () => {
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    const input = wrapper.get('[data-testid="general-editor-command"]')
    await input.setValue('nvim')
    await settle()

    expect(mocks.SetEditor).toHaveBeenCalledWith('nvim')
    wrapper.unmount()
  })

  // Every keystroke emits, and each save is a read-modify-write of
  // settings.yaml; only the value the typing settled on should reach disk.
  it('writes once for a typed command rather than once per keystroke', async () => {
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    const input = wrapper.get('[data-testid="general-editor-command"]')
    await input.setValue('n')
    await input.setValue('nv')
    await input.setValue('nvim')
    await settle()

    expect(mocks.SetEditor).toHaveBeenCalledTimes(1)
    expect(mocks.SetEditor).toHaveBeenCalledWith('nvim')
    wrapper.unmount()
  })

  it('restores the last accepted command when the save is rejected', async () => {
    mocks.EditorSettings.mockResolvedValue(editorSettings({ command: 'zed' }))
    mocks.SetEditor.mockRejectedValue(new Error('the editor command must be a single word, without flags'))
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="general-editor-command"]').setValue('code --wait')
    await settle()

    expect(wrapper.get('[data-testid="general-editor-command"]').attributes('value')).toBe('zed')
    expect(wrapper.get('[data-testid="general-error"]').text()).toContain('single word')
    wrapper.unmount()
  })

  // Leaving the pane inside the debounce window would otherwise drop the edit.
  it('flushes a pending write when the pane goes away', async () => {
    const wrapper = mount(GeneralSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="general-editor-command"]').setValue('nvim')
    wrapper.unmount()
    await settle()

    expect(mocks.SetEditor).toHaveBeenCalledWith('nvim')
  })
})
