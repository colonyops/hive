import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgentSessionsAll } from '../useAgentSessionsAll'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  allSessions: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: mocks.getAgentsEndpoint,
  createAgentWorkspacesClient: () => ({ allSessions: mocks.allSessions }),
}))

describe('useAgentSessionsAll', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
    mocks.allSessions.mockResolvedValue([{ id: 1, name: 'one' }])
  })

  it('reads every session through the shared client', async () => {
    const all = useAgentSessionsAll()
    expect(all.recentsLoaded.value).toBe(false)

    await all.reload()

    expect(all.recents.value.map((row) => row.id)).toEqual([1])
    expect(all.recentsLoaded.value).toBe(true)
    expect(all.recentsError.value).toBeNull()
  })

  it('keeps the last-good rows and reports a failed read', async () => {
    const all = useAgentSessionsAll()
    await all.reload()
    mocks.allSessions.mockRejectedValue(new Error('control plane down'))

    await all.reload()

    expect(all.recents.value.map((row) => row.id)).toEqual([1])
    expect(all.recentsError.value).toBe('control plane down')
  })

  it('stays unloaded when the Agents area is unavailable', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'gated off' })
    const all = useAgentSessionsAll()

    await all.reload()

    expect(mocks.allSessions).not.toHaveBeenCalled()
    expect(all.recentsLoaded.value).toBe(false)
  })
})
