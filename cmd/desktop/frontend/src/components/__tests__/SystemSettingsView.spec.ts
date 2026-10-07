import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SystemSettingsView from '../SystemSettingsView.vue'

const mocks = vi.hoisted(() => ({
  Info: vi.fn(),
  OpenHiveConfig: vi.fn(),
  OpenPath: vi.fn(),
  RevealPath: vi.fn(),
  ChooseDirectory: vi.fn(),
  SetDataDir: vi.fn(),
  SetConfigDir: vi.fn(),
  ClearDataDir: vi.fn(),
  ClearConfigDir: vi.fn(),
  Quit: vi.fn(),
  SetText: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  Info: mocks.Info,
  OpenHiveConfig: mocks.OpenHiveConfig,
  OpenPath: mocks.OpenPath,
  RevealPath: mocks.RevealPath,
  ChooseDirectory: mocks.ChooseDirectory,
  SetDataDir: mocks.SetDataDir,
  SetConfigDir: mocks.SetConfigDir,
  ClearDataDir: mocks.ClearDataDir,
  ClearConfigDir: mocks.ClearConfigDir,
  Quit: mocks.Quit,
}))
vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: mocks.SetText },
}))

const DATA = '/home/u/.local/share/hive'
const LOG = '/home/u/.local/share/hive/hive.log'
const DB = '/home/u/.local/share/hive/desktop/desktop-pipeline.db'

function info(overrides: Record<string, unknown> = {}) {
  return {
    dataDir: { path: DATA, exists: true, overridden: false },
    configDir: { path: '/home/u/.config/hive/desktop', exists: true, overridden: false },
    hiveDataDir: { path: DATA, exists: true, overridden: false },
    logFile: { path: LOG, exists: false, overridden: false },
    database: { path: DB, exists: true, overridden: false },
    agentWorkspaces: { path: '/home/u/.config/hive/desktop/workspaces', exists: true, overridden: false },
    hiveConfig: { path: '/home/u/.config/hive/config.yaml', exists: true, overridden: false },
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

describe('SystemSettingsView', () => {
  it('renders the resolved locations and hides Reset until overridden', async () => {
    mocks.Info.mockResolvedValue(info())
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="system-data-dir-path"]').text()).toBe(DATA)
    expect(wrapper.find('[data-testid="system-database-path"]').text()).toContain('desktop-pipeline.db')
    expect(wrapper.text()).toContain('Shared Hive data and logs may use another location')
    expect(wrapper.text()).toContain('Hive Desktop and the CLI append here')
    expect(wrapper.find('[data-testid="system-data-dir-reset"]').exists()).toBe(false)
  })

  it('opens and reveals a location through the service', async () => {
    mocks.Info.mockResolvedValue(info())
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="system-log-file-open"]').trigger('click')
    expect(mocks.OpenPath).toHaveBeenCalledWith(LOG)

    await wrapper.find('[data-testid="system-database-reveal"]').trigger('click')
    expect(mocks.RevealPath).toHaveBeenCalledWith(DB)
  })

  it('copies a path to the clipboard via the native runtime', async () => {
    mocks.SetText.mockResolvedValue(undefined)
    mocks.Info.mockResolvedValue(info())
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="system-data-dir-copy"]').trigger('click')
    await flushPromises()
    expect(mocks.SetText).toHaveBeenCalledWith(DATA)
    expect(wrapper.find('[data-testid="system-data-dir-copy"]').attributes('aria-label')).toBe('Copied')
  })

  it('changes the data directory and surfaces the restart banner and Reset', async () => {
    mocks.Info.mockResolvedValueOnce(info()).mockResolvedValueOnce(
      info({ dataDir: { path: '/icloud/hive', exists: true, overridden: true } }),
    )
    mocks.ChooseDirectory.mockResolvedValue('/icloud/hive')
    mocks.SetDataDir.mockResolvedValue(undefined)
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="system-restart-banner"]').exists()).toBe(false)

    await wrapper.find('[data-testid="system-data-dir-change"]').trigger('click')
    await flushPromises()

    expect(mocks.ChooseDirectory).toHaveBeenCalledWith('Choose data directory')
    expect(mocks.SetDataDir).toHaveBeenCalledWith('/icloud/hive')
    expect(wrapper.find('[data-testid="system-restart-banner"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="system-data-dir-reset"]').exists()).toBe(true)
  })

  it('does nothing when the directory picker is cancelled', async () => {
    mocks.Info.mockResolvedValue(info())
    mocks.ChooseDirectory.mockResolvedValue('')
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="system-config-dir-change"]').trigger('click')
    await flushPromises()

    expect(mocks.SetConfigDir).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="system-restart-banner"]').exists()).toBe(false)
  })

  it('quits the app from the restart banner', async () => {
    mocks.Info.mockResolvedValue(info({ dataDir: { path: '/icloud/hive', exists: true, overridden: true } }))
    mocks.ClearDataDir.mockResolvedValue(undefined)
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="system-data-dir-reset"]').trigger('click')
    await flushPromises()
    expect(mocks.ClearDataDir).toHaveBeenCalled()

    await wrapper.find('[data-testid="system-quit"]').trigger('click')
    expect(mocks.Quit).toHaveBeenCalled()
  })

  // The workspace root is a location like the others, but it belongs to the
  // Agents pane: System is the install, not everything with a path.
  it('leaves the agent-workspace root to the Agents pane', async () => {
    mocks.Info.mockResolvedValue(info())
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="agents-workspace-root"]').exists()).toBe(false)
  })
})
