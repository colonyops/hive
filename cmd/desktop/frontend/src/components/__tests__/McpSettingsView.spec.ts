import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import McpSettingsView from '../McpSettingsView.vue'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  mcpCatalogue: vi.fn(),
  SetText: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: mocks.getAgentsEndpoint,
  createAgentWorkspacesClient: () => ({ mcpCatalogue: mocks.mcpCatalogue }),
}))
vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: mocks.SetText },
}))

function entry(id: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    title: { 'hive-drive': 'Hive Drive', 'hive-canvas': 'Hive Canvas' }[id] ?? 'Hive Desktop',
    description: `${id} description`,
    shipped: true,
    stability: 'stable',
    shadows: '',
    transport: 'http',
    command: `http://127.0.0.1:4821/mcp${{ 'hive-drive': '/drive', 'hive-canvas': '/canvas' }[id] ?? ''}`,
    problem: '',
    ...overrides,
  }
}

async function mountView() {
  const wrapper = mount(McpSettingsView)
  await flushPromises()
  return wrapper
}

describe('McpSettingsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
    mocks.SetText.mockResolvedValue(undefined)
    mocks.mcpCatalogue.mockResolvedValue([
      entry('hive-canvas'),
      entry('hive-desktop'),
      entry('hive-drive'),
      entry('playwright', { transport: 'stdio', command: 'npx -y @playwright/mcp' }),
      // A user entry that shadows a shipped id launches the user's command,
      // not this app's server.
      entry('hive-drive', { shipped: false, command: 'http://elsewhere.test/mcp' }),
    ])
  })

  it('lists the servers this app hosts with their addresses, the drive first and its older address last', async () => {
    const wrapper = await mountView()

    expect(wrapper.get('[data-testid="settings-mcp-hive-drive-url"]').text()).toBe('http://127.0.0.1:4821/mcp/drive')
    expect(wrapper.get('[data-testid="settings-mcp-hive-desktop-url"]').text()).toBe('http://127.0.0.1:4821/mcp')
    expect(wrapper.get('[data-testid="settings-mcp-hive-canvas-url"]').text()).toBe('http://127.0.0.1:4821/mcp/canvas')
    expect(wrapper.find('[data-testid="settings-mcp-playwright"]').exists()).toBe(false)
    const titles = wrapper.findAll('section').map((section) => section.attributes('data-testid'))
    expect(titles.filter((id) => id?.startsWith('settings-mcp-hive-'))).toEqual([
      'settings-mcp-hive-drive',
      'settings-mcp-hive-desktop',
      'settings-mcp-hive-canvas',
    ])
  })

  it('copies the address and the setup for the chosen agent', async () => {
    const wrapper = await mountView()

    await wrapper.get('[data-testid="settings-mcp-hive-drive-url-copy"]').trigger('click')
    await flushPromises()
    expect(mocks.SetText).toHaveBeenLastCalledWith('http://127.0.0.1:4821/mcp/drive')

    expect(wrapper.get('[data-testid="settings-mcp-hive-drive-setup"]').text()).toBe(
      'claude mcp add --scope user --transport http hive-drive http://127.0.0.1:4821/mcp/drive',
    )

    await wrapper.get('[data-testid="settings-mcp-agent-codex"]').trigger('click')
    expect(wrapper.get('[data-testid="settings-mcp-hive-drive-setup"]').text()).toContain('[mcp_servers.hive-drive]')

    await wrapper.get('[data-testid="settings-mcp-hive-drive-setup-copy"]').trigger('click')
    await flushPromises()
    expect(mocks.SetText).toHaveBeenLastCalledWith('[mcp_servers.hive-drive]\nurl = "http://127.0.0.1:4821/mcp/drive"')
  })

  it('says why a server has no address instead of offering setup for it', async () => {
    mocks.mcpCatalogue.mockResolvedValue([
      entry('hive-canvas', { command: '', problem: 'The local HTTP server is disabled.' }),
    ])
    const wrapper = await mountView()

    expect(wrapper.get('[data-testid="settings-mcp-hive-canvas-problem"]').text()).toContain(
      'The local HTTP server is disabled.',
    )
    expect(wrapper.find('[data-testid="settings-mcp-hive-canvas-setup"]').exists()).toBe(false)
  })

  it('says why when the catalogue cannot be read in this build', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'tmux is not installed.' })
    const wrapper = await mountView()

    expect(wrapper.get('[data-testid="settings-mcp-unavailable"]').text()).toContain('tmux is not installed.')
  })
})
