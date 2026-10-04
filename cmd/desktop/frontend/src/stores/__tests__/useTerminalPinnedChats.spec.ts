import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { AgentSession } from '../../lib/agentWorkspacesClient'
import { useAgentSessionsAll } from '../useAgentSessionsAll'
import { useTerminalPinnedChats } from '../useTerminalPinnedChats'

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

function chat(id: number, name: string, terminalId = ''): AgentSession {
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
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://x', token: 't' })
  })

  it('turns a pinned chat into a sidebar row keyed on the slug the core declared', async () => {
    await listed([chat(7, 'api-refactor', 'agentws-7')])
    const { togglePin, rows, slugs } = useTerminalPinnedChats()

    togglePin(7)

    expect(rows.value).toEqual([
      { id: 'agentws-7', name: 'api-refactor', slug: 'agentws-7', repo: '', state: 'active', canvasOwner: '' },
    ])
    expect(slugs.value.has('agentws-7')).toBe(true)
  })

  // A stopped chat carries no terminalId, and it is exactly the chat a pin is
  // most useful for — the row has to exist for the pane to offer a resume.
  it('rows a pinned chat that is not running', async () => {
    await listed([chat(7, 'api-refactor')])
    const { togglePin, rows } = useTerminalPinnedChats()

    togglePin(7)

    expect(rows.value.map((row) => row.slug)).toEqual(['agentws-7'])
  })

  it('keeps pin order rather than the listing’s', async () => {
    await listed([chat(1, 'first'), chat(2, 'second'), chat(3, 'third')])
    const { togglePin, rows } = useTerminalPinnedChats()

    togglePin(3)
    togglePin(1)

    expect(rows.value.map((row) => row.name)).toEqual(['third', 'first'])
  })

  it('unpins by slug, which is what the Code view’s rows are keyed on', async () => {
    await listed([chat(7, 'api-refactor')])
    const { togglePin, unpinSlug, isPinned } = useTerminalPinnedChats()
    togglePin(7)

    unpinSlug('agentws-7')

    expect(isPinned(7)).toBe(false)
  })

  it('drops a pin whose chat the listing no longer carries', async () => {
    await listed([chat(7, 'api-refactor'), chat(8, 'docs-pass')])
    const { togglePin, pinnedIds } = useTerminalPinnedChats()
    togglePin(7)
    togglePin(8)
    await nextTick()

    await listed([chat(8, 'docs-pass')])
    await nextTick()

    expect(pinnedIds.value).toEqual([8])
  })

  // The pin set outlives a run; the listing does not. Pruning against a list
  // that has not loaded — or that is empty because the Agents area is gated
  // off — would silently discard every pin the user made.
  it('does not prune before the listing has loaded', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'gated off' })
    const { togglePin, pinnedIds } = useTerminalPinnedChats()
    togglePin(7)
    await nextTick()

    await useAgentSessionsAll().reload()
    await nextTick()

    expect(pinnedIds.value).toEqual([7])
  })
})
