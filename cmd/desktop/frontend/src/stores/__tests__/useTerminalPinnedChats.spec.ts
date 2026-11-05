import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentSession } from '../../lib/agentWorkspacesClient'
import { useAgentSessionsAll } from '../useAgentSessionsAll'
import { useTerminalPinnedChats } from '../useTerminalPinnedChats'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  allSessions: vi.fn(),
  pinSession: vi.fn(),
  importPins: vi.fn(),
  showToast: vi.fn(),
  events: new Map<string, () => void>(),
}))

vi.mock('../../composables/useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: () => void) => mocks.events.set(name, handler),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: mocks.getAgentsEndpoint,
  createAgentWorkspacesClient: () => ({
    allSessions: mocks.allSessions,
    pinSession: mocks.pinSession,
    importPins: mocks.importPins,
  }),
}))

vi.mock('../../composables/useToasts', () => ({ useToasts: () => ({ showToast: mocks.showToast }) }))

function chat(id: string, name: string, pinnedAt = 0, terminalId = ''): AgentSession {
  return {
    id,
    workspace: 'demo',
    name,
    agent: 'claude',
    lastOpenedAt: 0,
    slug: `agentws-${id}`,
    terminalId,
    windowId: '',
    paneId: '',
    cols: 0,
    rows: 0,
    resumeAttempted: false,
    notice: '',
    scheduleId: '',
    pinnedAt,
  }
}

// Loads the listing the way the Agents area does: through the shared client.
async function listed(sessions: AgentSession[]): Promise<void> {
  mocks.allSessions.mockResolvedValue(sessions)
  await useAgentSessionsAll().reload()
}

describe('useTerminalPinnedChats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
    mocks.pinSession.mockResolvedValue(undefined)
    mocks.importPins.mockResolvedValue(undefined)
  })

  it('turns a pinned chat into a sidebar row keyed on the slug the core declared', async () => {
    await listed([chat('7', 'api-refactor', 100, 'agentws-7'), chat('8', 'loose')])
    const { rows, slugs, isPinned } = useTerminalPinnedChats()

    expect(rows.value).toEqual([
      {
        server: '',
        id: 'agentws-7',
        name: 'api-refactor',
        slug: 'agentws-7',
        repo: '',
        repoIdentity: '',
        state: 'active',
        canvasOwner: '',
        tags: [],
        createdAt: '',
      },
    ])
    expect(slugs.value.has('agentws-7')).toBe(true)
    expect(isPinned('8')).toBe(false)
  })

  it('keeps pin order rather than the listing’s', async () => {
    await listed([chat('1', 'first', 200), chat('2', 'second'), chat('3', 'third', 100)])
    const { rows } = useTerminalPinnedChats()

    expect(rows.value.map((row) => row.name)).toEqual(['third', 'first'])
  })

  it('pins and unpins through the chat record, then rereads the listing', async () => {
    await listed([chat('7', 'api-refactor')])
    const { togglePin, unpinSlug, isPinned } = useTerminalPinnedChats()

    mocks.allSessions.mockResolvedValue([chat('7', 'api-refactor', 100)])
    await togglePin('7')
    expect(mocks.pinSession).toHaveBeenCalledWith('7', true)
    expect(isPinned('7')).toBe(true)

    mocks.allSessions.mockResolvedValue([chat('7', 'api-refactor')])
    await unpinSlug('agentws-7')
    expect(mocks.pinSession).toHaveBeenLastCalledWith('7', false)
    expect(isPinned('7')).toBe(false)
  })

  it('says so when a pin fails', async () => {
    await listed([chat('7', 'api-refactor')])
    mocks.pinSession.mockRejectedValue(new Error('boom'))

    await useTerminalPinnedChats().togglePin('7')

    expect(mocks.showToast).toHaveBeenCalledWith(expect.any(String), { severity: 'error' })
  })

  it('imports the pins browser storage kept, once, and forgets them', async () => {
    localStorage.setItem('hive.terminal.sidebar.pinned-chats', JSON.stringify([7, 'abc']))
    useTerminalPinnedChats()

    await listed([chat('abc', 'docs')])
    await vi.waitFor(() => expect(mocks.importPins).toHaveBeenCalledWith(['7', 'abc']))
    await vi.waitFor(() => expect(localStorage.getItem('hive.terminal.sidebar.pinned-chats')).toBeNull())

    await listed([chat('abc', 'docs', 100)])
    expect(mocks.importPins).toHaveBeenCalledTimes(1)
  })

  it('pins a chat a background job creates once the listing carries it', async () => {
    await listed([chat('1', 'triage')])
    const { pinWhenListed } = useTerminalPinnedChats()

    pinWhenListed('demo', 'triage')
    mocks.allSessions.mockResolvedValue([chat('1', 'triage'), chat('2', 'triage')])
    mocks.events.get('jobs:updated')?.()
    await vi.waitFor(() => expect(mocks.pinSession).toHaveBeenCalledWith('2', true))
  })

  it('does not reload the listing on job updates when no pin is waiting', async () => {
    await listed([])
    useTerminalPinnedChats()
    mocks.allSessions.mockClear()

    mocks.events.get('jobs:updated')?.()

    expect(mocks.allSessions).not.toHaveBeenCalled()
  })

  it('rereads the listing when a chat changes outside this window', async () => {
    await listed([chat('7', 'api-refactor')])
    const { isPinned } = useTerminalPinnedChats()

    mocks.allSessions.mockResolvedValue([chat('7', 'api-refactor', 100)])
    mocks.events.get('chats:updated')?.()

    await vi.waitFor(() => expect(isPinned('7')).toBe(true))
  })
})
