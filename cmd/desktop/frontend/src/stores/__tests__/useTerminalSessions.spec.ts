import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  groupTerminalSessions,
  terminalSessionGroups,
  useTerminalSessions,
  type TerminalSessionRow,
} from '../useTerminalSessions'

const mocks = vi.hoisted(() => ({ ListSessions: vi.fn(), Scratch: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  ListSessions: mocks.ListSessions,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice', () => ({
  Scratch: mocks.Scratch,
}))

describe('useTerminalSessions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Scratch.mockResolvedValue({ slug: 'Scratch', name: 'Terminals' })
  })

  it('loads every session the list carries, whatever its state', async () => {
    mocks.ListSessions.mockResolvedValue([
      {
        id: '1',
        name: 'fix the parser',
        slug: 'hive-fix-parser',
        repo: 'hay-kot/hive',
        state: 'active',
        canvasOwner: '',
        createdAt: '2026-10-01T12:00:00Z',
      },
      {
        id: '2',
        name: 'old work',
        slug: 'old-work',
        repo: 'hay-kot/hive',
        state: 'recycled',
        canvasOwner: '',
        createdAt: '2026-09-01T12:00:00Z',
      },
    ])
    const { sessions, loading, reload } = useTerminalSessions()

    await reload()

    expect(loading.value).toBe(false)
    // Filtering to what can be attached belongs to the tree, not the listing:
    // a recycled session is what the prune entry counts.
    expect(sessions.value.map((row) => row.slug)).toEqual(['hive-fix-parser', 'old-work'])
  })

  it('reads a null listing as no sessions', async () => {
    mocks.ListSessions.mockResolvedValue(null)
    const { sessions, reload } = useTerminalSessions()

    await reload()

    expect(sessions.value).toEqual([])
  })

  it('surfaces a listing failure and keeps the last-good rows', async () => {
    mocks.ListSessions.mockResolvedValue([
      {
        id: '1',
        name: 'fix the parser',
        slug: 'hive-fix-parser',
        repo: 'hay-kot/hive',
        state: 'active',
        canvasOwner: '',
      },
    ])
    const { sessions, error, reload } = useTerminalSessions()
    await reload()

    mocks.ListSessions.mockRejectedValue(new Error('hive.db is locked'))
    await reload()

    expect(error.value).toBe('hive.db is locked')
    // The reload is a revalidation of a tree that is already on screen; a
    // failure must not collapse it.
    expect(sessions.value.map((row) => row.slug)).toEqual(['hive-fix-parser'])
  })

  // The scratch terminal is not in hive's listing — the core declares it — but
  // it is a row of the same shape, so the tree needs no second kind of row.
  it('carries the scratch terminal as a row of its own, read once', async () => {
    mocks.ListSessions.mockResolvedValue([])
    const { scratch, reload } = useTerminalSessions()

    await reload()
    await reload()

    expect(scratch.value).toEqual({
      id: 'Scratch',
      name: 'Terminals',
      slug: 'Scratch',
      repo: '',
      state: 'active',
      canvasOwner: '',
      createdAt: '',
    })
    expect(mocks.Scratch).toHaveBeenCalledTimes(1)
  })

  it('lists the sessions anyway when the scratch terminal cannot be read', async () => {
    mocks.ListSessions.mockResolvedValue([
      {
        id: '1',
        name: 'fix the parser',
        slug: 'hive-fix-parser',
        repo: 'hay-kot/hive',
        state: 'active',
        canvasOwner: '',
      },
    ])
    mocks.Scratch.mockRejectedValue(new Error('the terminal is unavailable'))
    const { sessions, scratch, error, reload } = useTerminalSessions()

    await reload()

    expect(scratch.value).toBeNull()
    expect(error.value).toBeNull()
    expect(sessions.value.map((row) => row.slug)).toEqual(['hive-fix-parser'])
  })
})

describe('sessionRepository', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('resolves the remote of the session at a slug', async () => {
    mocks.ListSessions.mockResolvedValue([
      {
        id: '1',
        name: 'fix the parser',
        slug: 'hive-fix-parser',
        repo: 'https://github.com/hay-kot/hive.git',
        state: 'active',
        canvasOwner: '',
      },
    ])
    const { reload, sessionRepository } = useTerminalSessions()
    await reload()

    expect(sessionRepository('hive-fix-parser')).toBe('https://github.com/hay-kot/hive.git')
  })

  it('is empty for an unknown slug, no slug, and a session without a remote', async () => {
    mocks.ListSessions.mockResolvedValue([
      { id: '1', name: 'scratch', slug: 'scratch', repo: '', state: 'active', canvasOwner: '', createdAt: '' },
    ])
    const { reload, sessionRepository } = useTerminalSessions()
    await reload()

    expect(sessionRepository('scratch')).toBe('')
    expect(sessionRepository('gone')).toBe('')
    expect(sessionRepository('')).toBe('')
  })
})

describe('groupTerminalSessions', () => {
  function row(name: string, repo: string): TerminalSessionRow {
    return { id: name, name, slug: `slug-${name}`, repo, state: 'active', canvasOwner: '', createdAt: '' }
  }

  it('groups by remote with readable names, both levels alphabetical', () => {
    const groups = groupTerminalSessions([
      row('zeta', 'https://github.com/hay-kot/hive-desktop.git'),
      row('alpha', 'https://github.com/hay-kot/hive-desktop.git'),
      row('solo', 'git@github.com:colonyops/hive.git'),
    ])

    expect(groups.map((group) => group.name)).toEqual(['colonyops/hive', 'hay-kot/hive-desktop'])
    expect(groups[1].sessions.map((s) => s.name)).toEqual(['alpha', 'zeta'])
  })

  it('collects sessions without a remote under one label', () => {
    const groups = groupTerminalSessions([row('scratch', ''), row('local', '/Users/x/repos/scratchpad')])

    expect(groups.map((group) => group.name)).toEqual(['(no remote)', 'repos/scratchpad'])
  })
})

describe('terminalSessionGroups', () => {
  function row(name: string, repo: string): TerminalSessionRow {
    return { id: name, name, slug: `slug-${name}`, repo, state: 'active', canvasOwner: '', createdAt: '' }
  }

  it('pins the scratch terminal above the repositories, in a section of its own', () => {
    const scratch = row('Terminals', '')
    const groups = terminalSessionGroups([row('zeta', 'git@github.com:colonyops/hive.git')], scratch, [])

    // The section takes its name from the row, which is the heading it draws.
    expect(groups.map((group) => group.name)).toEqual(['Terminals', 'colonyops/hive'])
    expect(groups[0].kind).toBe('scratch')
    expect(groups[0].sessions).toEqual([scratch])
    // It is its own section rather than a session with no remote, which is
    // where the repo grouping would otherwise have put it.
    expect(groups[1].sessions.map((s) => s.name)).toEqual(['zeta'])
  })

  it('is the repositories alone when there is no scratch terminal', () => {
    const groups = terminalSessionGroups([row('zeta', 'git@github.com:colonyops/hive.git')], null, [])

    expect(groups.map((group) => group.name)).toEqual(['colonyops/hive'])
  })

  it('leads with the pinned chats, above the scratch terminal', () => {
    const chat = row('api-refactor', '')
    const groups = terminalSessionGroups([row('zeta', 'git@github.com:colonyops/hive.git')], row('Terminals', ''), [
      chat,
    ])

    expect(groups.map((group) => group.kind)).toEqual(['chats', 'scratch', 'repo'])
    expect(groups[0].name).toBe('Chats')
    expect(groups[0].sessions).toEqual([chat])
  })

  it('omits the chats section entirely when nothing is pinned', () => {
    const groups = terminalSessionGroups([row('zeta', 'git@github.com:colonyops/hive.git')], null, [])

    expect(groups.map((group) => group.kind)).toEqual(['repo'])
  })
})
