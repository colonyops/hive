import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useHiveSetup } from '../useHiveSetup'

const mocks = vi.hoisted(() => ({
  Setup: vi.fn(),
  Save: vi.fn(),
  InspectWorkspace: vi.fn(),
  ChooseDirectory: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hiveconfigservice', () => ({
  Setup: mocks.Setup,
  Save: mocks.Save,
  InspectWorkspace: mocks.InspectWorkspace,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  ChooseDirectory: mocks.ChooseDirectory,
}))

const AGENTS = [
  { name: 'claude', label: 'Claude Code', skipPermissionFlags: ['--dangerously-skip-permissions'], installed: true },
  { name: 'opencode', label: 'OpenCode', skipPermissionFlags: ['--agent', 'free-permissions-runner'], installed: false },
  { name: 'copilot', label: 'GitHub Copilot', skipPermissionFlags: [], installed: false },
]

function setup(config: Record<string, unknown> = {}) {
  return {
    config: {
      path: '/home/u/.config/hive/config.yaml',
      exists: true,
      usable: true,
      unreadable: '',
      defaultAgent: 'claude',
      profiles: [{ name: 'claude', command: 'claude', flags: [] }],
      workspaces: [{ path: '/home/u/code', exists: true, repos: 5 }],
      ...config,
    },
    agents: AGENTS,
    defaultAgentOverride: '',
  }
}

async function loaded(config: Record<string, unknown> = {}) {
  mocks.Setup.mockResolvedValue(setup(config))
  const hive = useHiveSetup()
  await hive.load()
  return hive
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Save.mockImplementation(async () => setup())
})

describe('useHiveSetup', () => {
  it('starts a first run on an installed agent, and on nothing when none are', async () => {
    const seeded = await loaded({ exists: false, usable: false, defaultAgent: '', profiles: [], workspaces: [] })
    expect(seeded.defaultAgent.value).toBe('claude')
    expect([...seeded.selectedAgents.value]).toEqual(['claude'])

    mocks.Setup.mockResolvedValue({ ...setup({ exists: false, usable: false, defaultAgent: '', profiles: [], workspaces: [] }), agents: AGENTS.map(a => ({ ...a, installed: false })) })
    const empty = useHiveSetup()
    await empty.load()
    expect(empty.selectedAgents.value.size).toBe(0)
    expect(empty.canSave.value).toBe(false)
  })

  // Removing the chosen default would write a config hive refuses to load,
  // which stops the next launch rather than degrading it.
  it('never leaves the default agent naming a profile it dropped', async () => {
    const hive = await loaded({
      defaultAgent: 'opencode',
      profiles: [
        { name: 'claude', command: 'claude', flags: [] },
        { name: 'opencode', command: 'opencode', flags: [] },
      ],
    })

    hive.toggleAgent(AGENTS[1], false)

    expect(hive.defaultAgent.value).toBe('claude')
    expect(hive.canSave.value).toBe(true)
  })

  // A hand-written profile is the case a naive "rewrite what the form holds"
  // save would silently delete.
  it('carries a profile it has no control for through a save untouched', async () => {
    const hive = await loaded({
      defaultAgent: 'fable',
      profiles: [
        { name: 'fable', command: 'claude --model fable', flags: ['--verbose'] },
        { name: 'claude', command: 'claude', flags: [] },
      ],
    })
    expect(hive.customProfiles.value.map(p => p.name)).toEqual(['fable'])

    hive.setSkipPermissions(true)
    await hive.save()

    expect(mocks.Save).toHaveBeenCalledWith({
      defaultAgent: 'fable',
      profiles: [
        // The custom profile keeps its own flags: the toggle is about the
        // presets this app offers, not about rewriting someone's command line.
        { name: 'fable', command: 'claude --model fable', flags: ['--verbose'] },
        { name: 'claude', command: 'claude', flags: ['--dangerously-skip-permissions'] },
      ],
      workspaces: ['/home/u/code'],
    })
  })

  it('reflects the flags already in the file rather than defaulting the toggle off', async () => {
    const on = await loaded({ profiles: [{ name: 'claude', command: 'claude', flags: ['--dangerously-skip-permissions'] }] })
    expect(on.skipPermissions.value).toBe(true)

    const off = await loaded()
    expect(off.skipPermissions.value).toBe(false)

    // A hand-written profile's own flags say nothing about this toggle.
    const custom = await loaded({
      defaultAgent: 'fable',
      profiles: [{ name: 'fable', command: 'claude --model fable', flags: ['--verbose'] }],
    })
    expect(custom.skipPermissions.value).toBe(false)
  })

  it('drops the flags again when the toggle goes back off', async () => {
    const hive = await loaded({ profiles: [{ name: 'claude', command: 'claude', flags: ['--dangerously-skip-permissions'] }] })

    hive.setSkipPermissions(false)

    expect(hive.profiles.value).toEqual([{ name: 'claude', command: 'claude', flags: [] }])
  })

  // Only the agent's own skip flags say anything about the toggle. Other
  // flags on the same profile neither light it up nor get deleted by it.
  it('tells the skip-permissions flag apart from the flags around it', async () => {
    const hive = await loaded({ profiles: [{ name: 'claude', command: 'claude', flags: ['--model', 'opus'] }] })
    expect(hive.skipPermissions.value).toBe(false)

    hive.setSkipPermissions(true)
    expect(hive.profiles.value[0].flags).toEqual(['--model', 'opus', '--dangerously-skip-permissions'])

    hive.setSkipPermissions(false)
    expect(hive.profiles.value[0].flags).toEqual(['--model', 'opus'])

    // A two-token skip flag is one run: an unrelated --agent survives it.
    const two = await loaded({
      defaultAgent: 'opencode',
      profiles: [{ name: 'opencode', command: 'opencode', flags: ['--agent', 'mine', '--agent', 'free-permissions-runner'] }],
    })
    expect(two.skipPermissions.value).toBe(true)
    two.setSkipPermissions(false)
    expect(two.profiles.value[0].flags).toEqual(['--agent', 'mine'])
  })

  it('reports when the read has finished, whether or not it succeeded', async () => {
    const ok = useHiveSetup()
    expect(ok.loaded.value).toBe(false)
    mocks.Setup.mockResolvedValue(setup())
    await ok.load()
    expect(ok.loaded.value).toBe(true)

    mocks.Setup.mockRejectedValue(new Error('boom'))
    const failed = useHiveSetup()
    await failed.load()
    expect(failed.loaded.value).toBe(true)
    expect(failed.setup.value).toBeNull()
  })

  // The backend refuses to rewrite a file it could not parse, so a save
  // button for one is a save button that always fails.
  it('cannot save a config that did not parse', async () => {
    const hive = await loaded({ unreadable: 'yaml: line 3: did not find expected key' })

    expect(hive.canSave.value).toBe(false)
  })

  it('holds the save while a typed folder is still being checked', async () => {
    const hive = await loaded()
    let settle: (w: { path: string, exists: boolean, repos: number }) => void = () => {}
    mocks.InspectWorkspace.mockReturnValue(new Promise((resolve) => { settle = resolve }))

    const adding = hive.addWorkspacePath('/home/u/more')
    expect(hive.canSave.value).toBe(false)

    settle({ path: '/home/u/more', exists: true, repos: 1 })
    await adding
    expect(hive.canSave.value).toBe(true)
    expect(hive.workspaces.value.map(w => w.path)).toEqual(['/home/u/code', '/home/u/more'])
  })

  // An agent with no known flag must not get an empty toggle that writes
  // nothing — selecting it produces a plain profile either way.
  it('leaves an agent with no skip-permission flag alone', async () => {
    const hive = await loaded({ exists: false, usable: false, defaultAgent: '', profiles: [], workspaces: [] })

    hive.toggleAgent(AGENTS[2], true)
    hive.setSkipPermissions(true)

    expect(hive.profiles.value.find(p => p.name === 'copilot')).toEqual({ name: 'copilot', command: 'copilot', flags: [] })
  })

  it('ignores a folder already in the list', async () => {
    const hive = await loaded()
    mocks.InspectWorkspace.mockResolvedValue({ path: '/home/u/code', exists: true, repos: 5 })

    await hive.addWorkspacePath('/home/u/code')

    expect(mocks.InspectWorkspace).not.toHaveBeenCalled()
    expect(hive.workspaces.value).toHaveLength(1)
  })

  it('adds nothing when the folder picker is cancelled', async () => {
    const hive = await loaded()
    mocks.ChooseDirectory.mockResolvedValue('')

    await hive.addWorkspace()

    expect(mocks.InspectWorkspace).not.toHaveBeenCalled()
    expect(hive.workspaces.value).toHaveLength(1)
  })

  it('reports a failed save without discarding the draft', async () => {
    const hive = await loaded()
    mocks.Save.mockRejectedValue(new Error('saving the Hive configuration: permission denied'))
    hive.toggleAgent(AGENTS[1], true)

    expect(await hive.save()).toBe(false)
    expect(hive.error.value).toContain('permission denied')
    expect(hive.selectedAgents.value.has('opencode')).toBe(true)
  })

  it('leaves the draft empty when the config could not be read', async () => {
    mocks.Setup.mockRejectedValue(new Error('no Hive config path is available'))
    const hive = useHiveSetup()

    await hive.load()

    expect(hive.setup.value).toBeNull()
    expect(hive.error.value).toContain('no Hive config path')
    expect(hive.canSave.value).toBe(false)
  })
})

describe('useHiveSetup — typed paths', () => {
  it('validates a typed path the same way the picker does', async () => {
    const hive = await loaded({ workspaces: [], usable: false })
    mocks.InspectWorkspace.mockResolvedValue({ path: '~/code', exists: true, repos: 4 })

    await hive.addWorkspacePath('  ~/code  ')

    expect(mocks.InspectWorkspace).toHaveBeenCalledWith('~/code')
    expect(hive.workspaces.value).toEqual([{ path: '~/code', exists: true, repos: 4 }])
  })

  it('ignores an empty typed path', async () => {
    const hive = await loaded()

    await hive.addWorkspacePath('   ')

    expect(mocks.InspectWorkspace).not.toHaveBeenCalled()
  })
})
