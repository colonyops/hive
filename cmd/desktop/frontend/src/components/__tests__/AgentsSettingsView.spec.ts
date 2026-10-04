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

const settingsBindings = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  SetCanvasFontSize: vi.fn(),
  SetCanvasLineSpacing: vi.fn(),
}))
vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settingsBindings,
)

const agents = vi.hoisted(() => ({ Available: vi.fn(), getAgentsEndpoint: vi.fn(), mcpCatalogue: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: agents.Available,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: agents.getAgentsEndpoint,
  createAgentWorkspacesClient: () => ({ mcpCatalogue: agents.mcpCatalogue }),
}))

beforeEach(() => {
  vi.clearAllMocks()
  agents.Available.mockResolvedValue({ available: true, reason: '' })
  agents.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
  agents.mcpCatalogue.mockResolvedValue([
    { id: 'hive-desktop', shipped: true, transport: 'http', command: 'http://127.0.0.1:4821/mcp', problem: '' },
    { id: 'hive-canvas', shipped: true, transport: 'http', command: 'http://127.0.0.1:4821/mcp/canvas', problem: '' },
  ])
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

    expect(wrapper.get('[data-testid="settings-canvas-font-size-small"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="settings-canvas-line-spacing-compact"]').attributes('aria-pressed')).toBe('true')

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

  // The address is what a user pastes into an agent's own MCP configuration;
  // the app never writes it there.
  it('shows the canvas server address to copy into another agent', async () => {
    const wrapper = mount(AgentsSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-canvas-server-url"]').text()).toBe('http://127.0.0.1:4821/mcp/canvas')

    await wrapper.get('[data-testid="settings-canvas-server-copy"]').trigger('click')
    await flushPromises()
    expect(mocks.SetText).toHaveBeenCalledWith('http://127.0.0.1:4821/mcp/canvas')
  })

  it('says why when the server has no address', async () => {
    agents.mcpCatalogue.mockResolvedValue([
      {
        id: 'hive-canvas',
        shipped: true,
        transport: 'http',
        command: '',
        problem: 'The local HTTP server is disabled.',
      },
    ])
    const wrapper = mount(AgentsSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-canvas-server"]').text()).toContain('The local HTTP server is disabled.')
    expect(wrapper.find('[data-testid="settings-canvas-server-copy"]').exists()).toBe(false)
  })
})
