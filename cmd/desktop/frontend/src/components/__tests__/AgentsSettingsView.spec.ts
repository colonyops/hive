import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AgentsSettingsView from '../AgentsSettingsView.vue'

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

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Info.mockResolvedValue({
    dataDir: { path: '/home/u/.local/share/hive', exists: true, overridden: false },
    configDir: { path: '/home/u/.config/hive/desktop', exists: true, overridden: false },
    hiveDataDir: { path: '/home/u/.local/share/hive', exists: true, overridden: false },
    logFile: { path: '/home/u/.local/share/hive/hive.log', exists: true, overridden: false },
    database: { path: '/home/u/.local/share/hive/desktop/desktop-pipeline.db', exists: true, overridden: false },
    agentWorkspaces: { path: WORKSPACES, exists: true, overridden: false },
    hiveConfig: { path: '/home/u/.config/hive/config.yaml', exists: true, overridden: false },
  })
  document.body.innerHTML = ''
})

describe('AgentsSettingsView', () => {
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
