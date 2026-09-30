import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AgentsSettingsView from '../AgentsSettingsView.vue'
import { resetCanvasTypographyForTests } from '../../composables/useCanvasTypography'

const WORKSPACES = '/home/u/.config/hive/desktop/workspaces'

const mocks = vi.hoisted(() => ({
  Info: vi.fn(),
  OpenHiveConfig: vi.fn(),
  OpenPath: vi.fn(),
  RevealPath: vi.fn(),
  SetText: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  Info: mocks.Info,
  OpenHiveConfig: mocks.OpenHiveConfig,
  OpenPath: mocks.OpenPath,
  RevealPath: mocks.RevealPath,
  ChooseDirectory: vi.fn(),
  SetDataDir: vi.fn(),
  SetConfigDir: vi.fn(),
  ClearDataDir: vi.fn(),
  ClearConfigDir: vi.fn(),
  Quit: vi.fn(),
}))
vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: mocks.SetText },
}))

const settingsBindings = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => settingsBindings)

beforeEach(() => {
  vi.clearAllMocks()
  resetCanvasTypographyForTests()
  settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
  settingsBindings.SetCanvasFontSize.mockResolvedValue(undefined)
  settingsBindings.SetCanvasLineSpacing.mockResolvedValue(undefined)
  mocks.Info.mockResolvedValue({
    dataDir: { path: '/home/u/.local/share/hive', exists: true, overridden: false },
    configDir: { path: '/home/u/.config/hive/desktop', exists: true, overridden: false },
    logFile: { path: '/home/u/.local/share/hive/desktop/desktop.log', exists: true, overridden: false },
    database: { path: '/home/u/.local/share/hive/desktop/desktop-pipeline.db', exists: true, overridden: false },
    agentWorkspaces: { path: WORKSPACES, exists: true, overridden: false },
    hiveConfig: { path: '/home/u/.config/hive/config.yaml', exists: true, overridden: false },
  })
  document.body.innerHTML = ''
})

describe('AgentsSettingsView', () => {
  it('changes the global canvas text size and line spacing', async () => {
    settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: 'small', canvasLineSpacing: 'compact' })
    const wrapper = mount(AgentsSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-canvas-font-size-small"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="settings-canvas-line-spacing-compact"]').attributes('aria-selected')).toBe('true')

    await wrapper.get('[data-testid="settings-canvas-font-size-xl"]').trigger('click')
    await wrapper.get('[data-testid="settings-canvas-line-spacing-relaxed"]').trigger('click')
    await flushPromises()

    expect(settingsBindings.SetCanvasFontSize).toHaveBeenCalledWith('xl')
    expect(settingsBindings.SetCanvasLineSpacing).toHaveBeenCalledWith('relaxed')
    const previewStyle = (wrapper.get('[data-testid="settings-canvas-preview-content"]').element as HTMLElement).style
    expect(previewStyle.getPropertyValue('--hv-font-size')).toBe('18px')
    expect(previewStyle.getPropertyValue('--hv-line-height')).toBe('1.85')
  })

  // The sidebar's root-unavailable message points here, so the resolved root
  // has to be readable from this pane.
  it('shows the resolved workspace root and opens it', async () => {
    const wrapper = mount(AgentsSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="agents-workspace-root-path"]').text()).toBe(WORKSPACES)

    await wrapper.get('[data-testid="agents-workspace-root-open"]').trigger('click')
    expect(mocks.OpenPath).toHaveBeenCalledWith(WORKSPACES)

    await wrapper.get('[data-testid="agents-workspace-root-reveal"]').trigger('click')
    expect(mocks.RevealPath).toHaveBeenCalledWith(WORKSPACES)
  })
})
