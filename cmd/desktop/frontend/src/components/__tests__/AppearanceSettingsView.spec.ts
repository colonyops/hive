import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppearanceSettingsView from '../AppearanceSettingsView.vue'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  Fonts: vi.fn(),
  SetInterfaceScale: vi.fn(),
  SetDocumentScale: vi.fn(),
  SetCodeScale: vi.fn(),
  SetTerminalScale: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => mocks)

describe('AppearanceSettingsView text size', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.AppearanceSettings.mockResolvedValue({ terminalScale: 110 })
    mocks.Fonts.mockResolvedValue({ all: [], monospace: [] })
    mocks.SetInterfaceScale.mockResolvedValue(undefined)
    mocks.SetTerminalScale.mockResolvedValue(undefined)
  })

  const value = (wrapper: ReturnType<typeof mount>, surface: string): string =>
    wrapper.get(`[data-testid="settings-appearance-scale-${surface}-stepper-value"]`).text()

  it('shows every surface as a percent', async () => {
    const wrapper = mount(AppearanceSettingsView)
    await flushPromises()

    expect(value(wrapper, 'interface')).toBe('100%')
    expect(value(wrapper, 'document')).toBe('100%')
    expect(value(wrapper, 'code')).toBe('100%')
    expect(value(wrapper, 'terminal')).toBe('110%')
  })

  it('steps a surface through the presets and saves it', async () => {
    const wrapper = mount(AppearanceSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="settings-appearance-scale-interface-stepper-increase"]').trigger('click')
    await flushPromises()

    expect(value(wrapper, 'interface')).toBe('110%')
    expect(mocks.SetInterfaceScale).toHaveBeenCalledWith(110)
    expect(mocks.SetTerminalScale).not.toHaveBeenCalled()
  })
})
