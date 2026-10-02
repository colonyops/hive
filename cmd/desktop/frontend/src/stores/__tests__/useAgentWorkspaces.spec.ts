import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgentWorkspaces } from '../useAgentWorkspaces'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  workspaces: vi.fn(),
  deleteWorkspace: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: mocks.getAgentsEndpoint,
  createAgentWorkspacesClient: () => ({ workspaces: mocks.workspaces, deleteWorkspace: mocks.deleteWorkspace }),
}))

function workspace(dir: string) {
  return {
    dir,
    name: dir,
    command: 'claude',
    danger: false,
    mcps: [],
    skills: [],
    schedules: [],
    problem: '',
    notice: '',
  }
}

function payload(dirs: string[]) {
  return {
    root: '/root',
    rootProblem: '',
    available: true,
    error: '',
    editor: { command: 'code', title: 'VS Code' },
    presets: [],
    workspaces: dirs.map(workspace),
  }
}

describe('useAgentWorkspaces', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
    mocks.workspaces.mockResolvedValue(payload(['a']))
    mocks.deleteWorkspace.mockResolvedValue(undefined)
  })

  it('probes once and holds the client it minted', async () => {
    const agents = useAgentWorkspaces()
    expect(agents.checking.value).toBe(true)

    await Promise.all([agents.ready(), agents.ready()])

    expect(mocks.Available).toHaveBeenCalledTimes(1)
    expect(agents.checking.value).toBe(false)
    expect(agents.available.value).toBe(true)
    expect(agents.client.value).not.toBeNull()
  })

  it('reads the workspaces payload and keeps the last-good rows on failure', async () => {
    const agents = useAgentWorkspaces()

    await agents.reloadWorkspaces()
    expect(agents.workspaces.value.map((w) => w.dir)).toEqual(['a'])
    expect(agents.root.value).toBe('/root')
    expect(agents.editor.value.title).toBe('VS Code')
    expect(agents.workspacesLoaded.value).toBe(true)

    mocks.workspaces.mockRejectedValue(new Error('control plane down'))
    await agents.reloadWorkspaces()

    expect(agents.workspaces.value.map((w) => w.dir)).toEqual(['a'])
    expect(agents.workspacesError.value).toBe('control plane down')
  })

  it('leaves the lists untouched and refuses a first-run chat when the area is unavailable', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'no ptyterm' })
    const agents = useAgentWorkspaces()

    await agents.reloadWorkspaces()

    expect(agents.workspacesLoaded.value).toBe(false)
    expect(agents.client.value).toBeNull()
    await expect(agents.startFirstRunChat()).rejects.toThrow('no ptyterm')
  })

  it('folds a deleted workspace out of the list before it revalidates', async () => {
    const agents = useAgentWorkspaces()
    mocks.workspaces.mockResolvedValue(payload(['a', 'b']))
    await agents.reloadWorkspaces()
    mocks.workspaces.mockResolvedValue(payload(['b']))

    await agents.deleteWorkspace('a')

    expect(mocks.deleteWorkspace).toHaveBeenCalledWith('a')
    expect(agents.workspaces.value.map((w) => w.dir)).toEqual(['b'])
  })
})
