import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgentWorkspaces } from '../useAgentWorkspaces'
import { useWorkspaceCanvases } from '../useWorkspaceCanvases'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  workspaces: vi.fn(),
  canvases: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: mocks.getAgentsEndpoint,
  createAgentWorkspacesClient: () => ({ workspaces: mocks.workspaces, canvases: mocks.canvases }),
}))

function workspaceList(...dirs: string[]) {
  return {
    root: '',
    rootProblem: '',
    editor: { command: '', title: '' },
    presets: [],
    workspaces: dirs.map((dir) => ({ dir, name: dir })),
  }
}

describe('useWorkspaceCanvases', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
    mocks.workspaces.mockResolvedValue(workspaceList('web-app', 'docs'))
    mocks.canvases.mockImplementation((workspace: string) =>
      Promise.resolve(workspace === 'web-app' ? [{ workspace, name: 'plan' }] : [{ workspace, name: 'perf-report' }]),
    )
  })

  it('does not read until asked', () => {
    useWorkspaceCanvases()
    expect(mocks.Available).not.toHaveBeenCalled()
  })

  it("lists every workspace's canvases in one list", async () => {
    await useAgentWorkspaces().reloadWorkspaces()
    const all = useWorkspaceCanvases()

    await all.reload()

    expect(all.canvases.value.map((meta) => `${meta.workspace}/${meta.name}`)).toEqual([
      'web-app/plan',
      'docs/perf-report',
    ])
  })

  it('keeps the last-good rows and reports a failed read', async () => {
    await useAgentWorkspaces().reloadWorkspaces()
    const all = useWorkspaceCanvases()
    await all.reload()
    mocks.canvases.mockRejectedValue(new Error('control plane down'))

    await all.reload()

    expect(all.canvases.value).toHaveLength(2)
    expect(all.error.value).toBe('control plane down')
  })

  it('lists nothing when the Agents area is unavailable', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'gated off' })
    const all = useWorkspaceCanvases()

    await all.reload()

    expect(mocks.canvases).not.toHaveBeenCalled()
    expect(all.canvases.value).toEqual([])
  })
})
