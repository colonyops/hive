import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import HiveSettingsView from '../HiveSettingsView.vue'
import { useCommandPalette } from '../../composables/useCommands'

const mocks = vi.hoisted(() => ({
  Info: vi.fn(),
  OpenHiveConfig: vi.fn(),
  OpenPath: vi.fn(),
  RevealPath: vi.fn(),
  ChooseDirectory: vi.fn(),
  OpenURL: vi.fn(),
  SetText: vi.fn(),
  Setup: vi.fn(),
  Save: vi.fn(),
  InspectWorkspace: vi.fn(),
  CommandStatus: vi.fn(),
  SetInstall: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  Info: mocks.Info,
  OpenHiveConfig: mocks.OpenHiveConfig,
  OpenPath: mocks.OpenPath,
  RevealPath: mocks.RevealPath,
  ChooseDirectory: mocks.ChooseDirectory,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hiveconfigservice', () => ({
  Setup: mocks.Setup,
  Save: mocks.Save,
  InspectWorkspace: mocks.InspectWorkspace,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hivecliservice', () => ({
  Status: mocks.CommandStatus,
  SetInstall: mocks.SetInstall,
}))
vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: mocks.OpenURL },
  Clipboard: { SetText: mocks.SetText },
}))

const CONFIG = '/home/u/.config/hive/config.yaml'
const WORKSPACE = '/home/u/code'

function info(exists: boolean, overridden = false) {
  return {
    dataDir: { path: '/home/u/.local/share/hive', exists: true, overridden: false },
    configDir: { path: '/home/u/.config/hive/desktop', exists: true, overridden: false },
    logFile: { path: '/home/u/.local/share/hive/desktop/desktop.log', exists: true, overridden: false },
    database: { path: '/home/u/.local/share/hive/desktop/desktop-pipeline.db', exists: true, overridden: false },
    agentWorkspaces: { path: '/home/u/.config/hive/desktop/workspaces', exists: true, overridden: false },
    hiveConfig: { path: CONFIG, exists, overridden },
  }
}

function setup(
  over: Partial<{
    exists: boolean
    usable: boolean
    unreadable: string
    defaultAgent: string
    profiles: Array<{ name: string; command: string; flags: string[] | null }>
    workspaces: Array<{ path: string; exists: boolean; repos: number }>
    defaultAgentOverride: string
  }> = {},
) {
  return {
    config: {
      path: CONFIG,
      exists: over.exists ?? true,
      usable: over.usable ?? true,
      unreadable: over.unreadable ?? '',
      defaultAgent: over.defaultAgent ?? 'claude',
      profiles: over.profiles ?? [{ name: 'claude', command: 'claude', flags: [] }],
      workspaces: over.workspaces ?? [{ path: WORKSPACE, exists: true, repos: 12 }],
    },
    agents: [
      {
        name: 'claude',
        label: 'Claude Code',
        skipPermissionFlags: ['--dangerously-skip-permissions'],
        installed: true,
      },
      {
        name: 'opencode',
        label: 'OpenCode',
        skipPermissionFlags: ['--agent', 'free-permissions-runner'],
        installed: false,
      },
      { name: 'copilot', label: 'GitHub Copilot', skipPermissionFlags: [], installed: false },
    ],
    defaultAgentOverride: over.defaultAgentOverride ?? '',
  }
}

const LINK = '/home/u/.local/bin/hive'
const APP_EXE = '/Applications/Hive.app/Contents/MacOS/hive-desktop'

function commandStatus(
  over: Partial<{
    enabled: boolean
    linked: boolean
    foreign: boolean
    resolved: string
    commandVersion: string
    linkDirOnPath: boolean
    unsupported: string
  }> = {},
) {
  const linked = over.linked ?? over.enabled ?? false
  const exists = linked || (over.foreign ?? false)
  const resolved = over.resolved ?? (linked ? LINK : '')
  const commandVersion = over.commandVersion ?? (resolved ? '0.61.0' : '')
  return {
    asked: over.enabled !== undefined,
    enabled: over.enabled ?? false,
    unsupported: over.unsupported ?? '',
    link: { path: LINK, exists, appOwned: linked, target: linked ? APP_EXE : '' },
    conflict: (over.enabled ?? false) && (over.foreign ?? false),
    linkDir: '/home/u/.local/bin',
    linkDirOnPath: over.linkDirOnPath ?? true,
    resolved,
    shadowed: linked && resolved !== LINK,
    appVersion: '0.61.0',
    commandVersion,
    versionsDiffer: commandVersion !== '' && commandVersion !== '0.61.0',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.CommandStatus.mockResolvedValue(commandStatus())
  mocks.SetInstall.mockImplementation((install: boolean) => Promise.resolve(commandStatus({ enabled: install })))
  mocks.OpenHiveConfig.mockResolvedValue(undefined)
  mocks.OpenPath.mockResolvedValue(undefined)
  mocks.RevealPath.mockResolvedValue(undefined)
  mocks.Setup.mockResolvedValue(setup())
  mocks.Save.mockImplementation(async () => setup())
  mocks.InspectWorkspace.mockResolvedValue({ path: WORKSPACE, exists: true, repos: 12 })
  mocks.ChooseDirectory.mockResolvedValue(WORKSPACE)
})

describe('HiveSettingsView', () => {
  // The pane points at the file and nothing more: first run is the only
  // writer, so every change here is a hand edit followed by a restart.
  it('sends edits to the file', async () => {
    mocks.Info.mockResolvedValue(info(true))
    const wrapper = mount(HiveSettingsView)
    await flushPromises()

    const notice = wrapper.get('[data-testid="hive-restart-notice"]').text()
    expect(notice).toContain('Edit this file in your own editor')
    expect(notice).toContain('restart the app')

    await wrapper.get('[data-testid="hive-cli-docs"]').trigger('click')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://colonyops.github.io/hive/')
  })

  it('offers no editor for the config', async () => {
    mocks.Info.mockResolvedValue(info(true))
    const wrapper = mount(HiveSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="hive-config-save"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="hive-setup-agents"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="hive-workspace-list"]').exists()).toBe(false)
    expect(mocks.Save).not.toHaveBeenCalled()
  })

  // A file the user hand-edited into something Hive cannot parse stops
  // sessions starting, and this pane is where they come to look.
  it('reports a config it could not parse', async () => {
    mocks.Info.mockResolvedValue(info(true))
    mocks.Setup.mockResolvedValue(setup({ unreadable: 'yaml: line 4: mapping values are not allowed', usable: false }))
    const wrapper = mount(HiveSettingsView)
    await flushPromises()

    const banner = wrapper.get('[data-testid="hive-unreadable"]').text()
    expect(banner).toContain('line 4')
    expect(banner).toContain('restart Hive Desktop')
  })

  it('opens and reveals an existing config', async () => {
    mocks.Info.mockResolvedValue(info(true, true))
    const wrapper = mount(HiveSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="hive-config-path"]').text()).toBe(CONFIG)
    expect(wrapper.get('[data-testid="hive-config-overridden"]').text()).toBe('HIVE_CONFIG')

    const palette = useCommandPalette()
    palette.query.value = ''
    palette.scope.value = 'actions'
    const actionIds = palette.results.value.map((row) => row.id)
    expect(actionIds).toContain('hive:config:copy')
    expect(actionIds).toContain('hive:config:open')
    expect(actionIds).toContain('hive:config:reveal')
    expect(actionIds).not.toContain('hive:config:create')

    await wrapper.get('[data-testid="hive-config-open"]').trigger('click')
    await wrapper.get('[data-testid="hive-config-reveal"]').trigger('click')

    expect(mocks.OpenPath).toHaveBeenCalledWith(CONFIG)
    expect(mocks.RevealPath).toHaveBeenCalledWith(CONFIG)
  })

  it('creates a missing config, opens it, and refreshes its state', async () => {
    mocks.Info.mockResolvedValueOnce(info(false)).mockResolvedValueOnce(info(true))
    mocks.Setup.mockResolvedValue(setup({ exists: false, usable: false, workspaces: [], profiles: [] }))
    const wrapper = mount(HiveSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="hive-config-missing"]').text()).toContain('built-in defaults')
    expect(wrapper.find('[data-testid="hive-config-open"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="hive-config-reveal"]').exists()).toBe(false)

    const palette = useCommandPalette()
    palette.query.value = ''
    palette.scope.value = 'actions'
    expect(palette.results.value.map((row) => row.id)).toEqual(
      expect.arrayContaining(['hive:config:copy', 'hive:config:create']),
    )
    expect(palette.results.value.map((row) => row.id)).not.toContain('hive:config:open')

    await wrapper.get('[data-testid="hive-config-create"]').trigger('click')
    await flushPromises()

    expect(mocks.OpenHiveConfig).toHaveBeenCalledOnce()
    expect(mocks.Info).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="hive-config-missing"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="hive-config-open"]').exists()).toBe(true)
    expect(palette.results.value.map((row) => row.id)).toContain('hive:config:open')
    expect(palette.results.value.map((row) => row.id)).not.toContain('hive:config:create')
  })

  it('refreshes file state while preserving an open failure', async () => {
    mocks.Info.mockResolvedValueOnce(info(false)).mockResolvedValueOnce(info(true))
    mocks.Setup.mockResolvedValue(setup({ exists: false, usable: false, workspaces: [], profiles: [] }))
    mocks.OpenHiveConfig.mockRejectedValue(new Error('the file was created but no editor opened'))
    const wrapper = mount(HiveSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="hive-config-create"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="hive-settings-error"]').text()).toContain('no editor opened')
    expect(wrapper.find('[data-testid="hive-config-missing"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="hive-config-open"]').exists()).toBe(true)
  })

  describe('hive command', () => {
    it('installs and removes the command from the switch', async () => {
      mocks.Info.mockResolvedValue(info(true))
      const wrapper = mount(HiveSettingsView)
      await flushPromises()

      await wrapper.get('[data-testid="hive-command-switch"]').trigger('click')
      await flushPromises()
      expect(mocks.SetInstall).toHaveBeenCalledWith(true)
      expect(wrapper.get('[data-testid="hive-command-version"]').text()).toContain(LINK)

      await wrapper.get('[data-testid="hive-command-switch"]').trigger('click')
      await flushPromises()
      expect(mocks.SetInstall).toHaveBeenLastCalledWith(false)
    })

    it('disables the switch in a build that cannot install it', async () => {
      mocks.Info.mockResolvedValue(info(true))
      mocks.CommandStatus.mockResolvedValue(commandStatus({ unsupported: 'This is a development build.' }))
      const wrapper = mount(HiveSettingsView)
      await flushPromises()

      expect(wrapper.get('[data-testid="hive-command-switch"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('[data-testid="hive-command-install"]').text()).toContain('development build')
    })

    it('reports a foreign file at the install path', async () => {
      mocks.Info.mockResolvedValue(info(true))
      mocks.CommandStatus.mockResolvedValue(commandStatus({ enabled: true, linked: false, foreign: true }))
      const wrapper = mount(HiveSettingsView)
      await flushPromises()

      expect(wrapper.get('[data-testid="hive-command-conflict"]').text()).toContain('left it alone')
    })

    it('reports a hive that runs before the app command, and the version gap', async () => {
      mocks.Info.mockResolvedValue(info(true))
      mocks.CommandStatus.mockResolvedValue(
        commandStatus({ enabled: true, resolved: '/opt/homebrew/bin/hive', commandVersion: 'v0.58.0' }),
      )
      const wrapper = mount(HiveSettingsView)
      await flushPromises()

      expect(wrapper.get('[data-testid="hive-command-shadowed"]').text()).toContain('/opt/homebrew/bin/hive')
      expect(wrapper.find('[data-testid="hive-command-version-differs"]').exists()).toBe(true)
      expect(wrapper.get('[data-testid="hive-command-version"]').text()).toContain('v0.58.0')
    })

    it('hints at PATH when the link directory is missing from it', async () => {
      mocks.Info.mockResolvedValue(info(true))
      mocks.CommandStatus.mockResolvedValue(commandStatus({ enabled: true, resolved: '', linkDirOnPath: false }))
      const wrapper = mount(HiveSettingsView)
      await flushPromises()

      expect(wrapper.get('[data-testid="hive-command-not-on-path"]').text()).toContain(
        'export PATH="/home/u/.local/bin:$PATH"',
      )
    })
  })
})
