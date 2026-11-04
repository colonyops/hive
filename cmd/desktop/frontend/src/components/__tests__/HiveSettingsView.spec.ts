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
  CLIStatus: vi.fn(),
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
  Status: mocks.CLIStatus,
  SetInstall: mocks.SetInstall,
}))
vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: mocks.OpenURL },
  Clipboard: { SetText: mocks.SetText },
}))

function command(over: Record<string, unknown> = {}, install: boolean | null = true) {
  return {
    install,
    command: {
      skip: '',
      detail: '',
      installPath: '/home/u/.local/bin/hive',
      owner: 'app',
      target: '/Applications/Hive.app/Contents/MacOS/hive-desktop',
      resolved: '/home/u/.local/bin/hive',
      shadowed: false,
      onPath: true,
      appVersion: '1.2.0',
      commandVersion: '1.2.0',
      differs: false,
      ...over,
    },
  }
}

const CONFIG = '/home/u/.config/hive/config.yaml'
const WORKSPACE = '/home/u/code'

function info(exists: boolean, overridden = false) {
  return {
    dataDir: { path: '/home/u/.local/share/hive', exists: true, overridden: false },
    configDir: { path: '/home/u/.config/hive/desktop', exists: true, overridden: false },
    hiveDataDir: { path: '/home/u/.local/share/hive', exists: true, overridden: false },
    logFile: { path: '/home/u/.local/share/hive/hive.log', exists: true, overridden: false },
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

beforeEach(() => {
  vi.clearAllMocks()
  mocks.OpenHiveConfig.mockResolvedValue(undefined)
  mocks.OpenPath.mockResolvedValue(undefined)
  mocks.RevealPath.mockResolvedValue(undefined)
  mocks.Setup.mockResolvedValue(setup())
  mocks.CLIStatus.mockResolvedValue(command())
  mocks.Save.mockImplementation(async () => setup())
  mocks.InspectWorkspace.mockResolvedValue({ path: WORKSPACE, exists: true, repos: 12 })
  mocks.ChooseDirectory.mockResolvedValue(WORKSPACE)
})

describe('HiveSettingsView', () => {
  // The pane points at the file and nothing more: first run is the only
  // writer, so every change here is a hand edit followed by a restart.
  it('points edits at the file', async () => {
    mocks.Info.mockResolvedValue(info(true))
    const wrapper = mount(HiveSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="hive-data-dir-path"]').text()).toBe('/home/u/.local/share/hive')
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

  describe('the hive command section', () => {
    async function mounted(status: ReturnType<typeof command>) {
      mocks.Info.mockResolvedValue(info(true))
      mocks.CLIStatus.mockResolvedValue(status)
      const wrapper = mount(HiveSettingsView)
      await flushPromises()
      return wrapper
    }

    it('shows an installed command that matches the app', async () => {
      const wrapper = await mounted(command())

      expect(wrapper.get('[data-testid="hive-command-install"]').attributes('aria-checked')).toBe('true')
      expect(wrapper.get('[data-testid="hive-command-path"]').text()).toBe('/home/u/.local/bin/hive')
      expect(wrapper.get('[data-testid="hive-command-owner"]').text()).toBe('Installed by Hive.')
      expect(wrapper.get('[data-testid="hive-command-app-version"]').text()).toBe('1.2.0')
      expect(wrapper.get('[data-testid="hive-command-version"]').text()).toContain('1.2.0')
      for (const id of ['differs', 'path-hint', 'shadowed']) {
        expect(wrapper.find(`[data-testid="hive-command-${id}"]`).exists()).toBe(false)
      }
    })

    it('turns the command on and off through the service', async () => {
      const wrapper = await mounted(command({ owner: 'none', target: '', resolved: '' }, null))
      expect(wrapper.get('[data-testid="hive-command-install"]').attributes('aria-checked')).toBe('false')

      mocks.SetInstall.mockResolvedValue(command())
      await wrapper.get('[data-testid="hive-command-install"]').trigger('click')
      await flushPromises()

      expect(mocks.SetInstall).toHaveBeenCalledWith(true)
      expect(wrapper.get('[data-testid="hive-command-install"]').attributes('aria-checked')).toBe('true')
    })

    it('warns when the command in the terminal is another version', async () => {
      const wrapper = await mounted(command({ commandVersion: '1.1.0', differs: true }))
      expect(wrapper.get('[data-testid="hive-command-differs"]').text()).toContain('not the version of this app')
    })

    it('gives the PATH line when the install directory is not on the login shell PATH', async () => {
      const wrapper = await mounted(command({ onPath: false, resolved: '', commandVersion: '' }))

      expect(wrapper.get('[data-testid="hive-command-path-hint"]').text()).toContain(
        'export PATH="$HOME/.local/bin:$PATH"',
      )
      expect(wrapper.get('[data-testid="hive-command-version"]').text()).toBe('not found')
      expect(wrapper.find('[data-testid="hive-command-differs"]').exists()).toBe(false)

      await wrapper.get('[data-testid="hive-command-copy-path"]').trigger('click')
      expect(mocks.SetText).toHaveBeenCalledWith('export PATH="$HOME/.local/bin:$PATH"')
    })

    it('names the copy that shadows the link and how to remove it', async () => {
      const wrapper = await mounted(
        command({ resolved: '/opt/homebrew/bin/hive', shadowed: true, commandVersion: '1.1.0', differs: true }),
      )

      const hint = wrapper.get('[data-testid="hive-command-shadowed"]').text()
      expect(hint).toContain('/opt/homebrew/bin/hive')
      expect(hint).toContain('brew uninstall --cask hive')
      expect(hint).toContain('go install')
    })

    it('leaves a hive it did not install alone and says so', async () => {
      const wrapper = await mounted(command({ owner: 'foreign', target: '/opt/homebrew/bin/hive' }, false))
      expect(wrapper.get('[data-testid="hive-command-owner"]').text()).toContain('Hive leaves it alone')
    })

    it('disables the switch for a development build and says why', async () => {
      const wrapper = await mounted(command({ skip: 'dev', owner: 'none', resolved: '' }, null))

      expect(wrapper.get('[data-testid="hive-command-install"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('[data-testid="hive-command-row"]').text()).toContain('Development builds')
    })

    it('tells a translocated app to move to Applications', async () => {
      const wrapper = await mounted(command({ skip: 'translocated', owner: 'none', resolved: '' }, true))

      expect(wrapper.get('[data-testid="hive-command-install"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('[data-testid="hive-command-row"]').text()).toContain('Applications folder')
    })

    it('shows why an unwritable directory could not take the link', async () => {
      const wrapper = await mounted(command({ skip: 'unwritable', detail: 'permission denied', owner: 'none' }))
      expect(wrapper.get('[data-testid="hive-command-unwritable"]').text()).toContain('permission denied')
    })
  })
})
