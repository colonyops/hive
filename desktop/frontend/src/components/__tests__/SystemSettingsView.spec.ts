import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SystemSettingsView from '../SystemSettingsView.vue'

const mocks = vi.hoisted(() => ({
  Info: vi.fn(),
  Build: vi.fn(),
  OpenPath: vi.fn(),
  RevealPath: vi.fn(),
  ChooseDirectory: vi.fn(),
  SetDataDir: vi.fn(),
  SetConfigDir: vi.fn(),
  ClearDataDir: vi.fn(),
  ClearConfigDir: vi.fn(),
  Quit: vi.fn(),
  SetText: vi.fn(),
  OpenURL: vi.fn(),
  Status: vi.fn(),
  SetEnabled: vi.fn(),
  CheckNow: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/desktop/systemservice', () => ({
  Info: mocks.Info,
  Build: mocks.Build,
  OpenPath: mocks.OpenPath,
  RevealPath: mocks.RevealPath,
  ChooseDirectory: mocks.ChooseDirectory,
  SetDataDir: mocks.SetDataDir,
  SetConfigDir: mocks.SetConfigDir,
  ClearDataDir: mocks.ClearDataDir,
  ClearConfigDir: mocks.ClearConfigDir,
  Quit: mocks.Quit,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/desktop/updaterservice', () => ({
  Status: mocks.Status,
  SetEnabled: mocks.SetEnabled,
  CheckNow: mocks.CheckNow,
}))
vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: mocks.SetText },
  Browser: { OpenURL: mocks.OpenURL },
}))

function updateInfo(overrides: Record<string, unknown> = {}) {
  return {
    enabled: true,
    available: false,
    currentVersion: '1.4.0',
    latestVersion: '',
    notes: '',
    releaseUrl: '',
    ...overrides,
  }
}

function buildInfo(overrides: Record<string, unknown> = {}) {
  return {
    version: '1.4.0',
    commit: 'abc1234',
    date: '2026-07-01T12:00:00Z',
    repoUrl: 'https://github.com/colonyops/hive',
    releaseUrl: 'https://github.com/colonyops/hive/releases/tag/desktop-v1.4.0',
    ...overrides,
  }
}

const DATA = '/home/u/.local/share/hive'
const LOG = '/home/u/.local/share/hive/desktop/desktop.log'
const DB = '/home/u/.local/share/hive/desktop/desktop-pipeline.db'

function info(overrides: Record<string, unknown> = {}) {
  return {
    dataDir: { path: DATA, exists: true, overridden: false },
    configDir: { path: '/home/u/.config/hive/desktop', exists: true, overridden: false },
    logFile: { path: LOG, exists: false, overridden: false },
    database: { path: DB, exists: true, overridden: false },
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Build.mockResolvedValue(buildInfo())
  mocks.Status.mockResolvedValue(updateInfo())
  mocks.SetEnabled.mockResolvedValue(undefined)
  mocks.CheckNow.mockResolvedValue(updateInfo())
  document.body.innerHTML = ''
})

describe('SystemSettingsView', () => {
  it('renders the resolved locations and hides Reset until overridden', async () => {
    mocks.Info.mockResolvedValue(info())
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="system-data-dir-path"]').text()).toBe(DATA)
    expect(wrapper.find('[data-testid="system-database-path"]').text()).toContain('desktop-pipeline.db')
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
    expect(wrapper.find('[data-testid="system-data-dir-copy"]').text()).toContain('Copied')
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

  it('renders the build info and links to the repo and GitHub release', async () => {
    mocks.Info.mockResolvedValue(info())
    mocks.OpenURL.mockResolvedValue(undefined)
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="system-build-version"]').text()).toBe('1.4.0')
    expect(wrapper.find('[data-testid="system-build-commit"]').text()).toBe('abc1234')
    expect(wrapper.find('[data-testid="system-build-date"]').text()).toBe('2026-07-01T12:00:00Z')

    await wrapper.find('[data-testid="system-build-repo"]').trigger('click')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://github.com/colonyops/hive')

    await wrapper.find('[data-testid="system-build-release"]').trigger('click')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://github.com/colonyops/hive/releases/tag/desktop-v1.4.0')
  })

  it('keeps the repo link but hides the release link for dev builds', async () => {
    mocks.Info.mockResolvedValue(info())
    mocks.Build.mockResolvedValue(buildInfo({ version: 'dev', releaseUrl: '' }))
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="system-build-version"]').text()).toBe('dev')
    expect(wrapper.find('[data-testid="system-build-repo"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="system-build-release"]').exists()).toBe(false)
  })

  it('toggles automatic updates through the service', async () => {
    mocks.Info.mockResolvedValue(info())
    mocks.Status.mockResolvedValue(updateInfo({ enabled: true }))
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="system-auto-update"]').trigger('click')
    expect(mocks.SetEnabled).toHaveBeenCalledWith(false)
  })

  it('checks for updates and shows an available result inline', async () => {
    mocks.Info.mockResolvedValue(info())
    mocks.CheckNow.mockResolvedValue(updateInfo({ available: true, latestVersion: '1.5.0' }))
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="system-check-update"]').trigger('click')
    await flushPromises()
    expect(mocks.CheckNow).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="system-update-available"]').text()).toContain('1.5.0')
  })

  it('shows up to date after a check finds nothing', async () => {
    mocks.Info.mockResolvedValue(info())
    mocks.CheckNow.mockResolvedValue(updateInfo({ available: false }))
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="system-check-update"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="system-update-uptodate"]').exists()).toBe(true)
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
})
