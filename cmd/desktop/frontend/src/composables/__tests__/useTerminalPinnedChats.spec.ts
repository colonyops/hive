import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { resetTerminalPinnedChatsForTests, useTerminalPinnedChats } from '../useTerminalPinnedChats'
import { resetAgentSessionsAllForTests, useAgentSessionsAll } from '../useAgentSessionsAll'
import type { AgentSession } from '../../lib/agentWorkspacesClient'

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

describe('useTerminalPinnedChats', () => {
  beforeEach(() => {
    localStorage.clear()
    resetAgentSessionsAllForTests()
    resetTerminalPinnedChatsForTests()
  })

  it('turns a pinned chat into a sidebar row keyed on the slug the core declared', () => {
    const { recents } = useAgentSessionsAll()
    recents.value = [chat(7, 'api-refactor', 'agentws-7')]
    const { togglePin, rows, slugs } = useTerminalPinnedChats()

    togglePin(7)

    expect(rows.value).toEqual([{ id: 'agentws-7', name: 'api-refactor', slug: 'agentws-7', repo: '', state: 'active' }])
    expect(slugs.value.has('agentws-7')).toBe(true)
  })

  // A stopped chat carries no terminalId, and it is exactly the chat a pin is
  // most useful for — the row has to exist for the pane to offer a resume.
  it('rows a pinned chat that is not running', () => {
    const { recents } = useAgentSessionsAll()
    recents.value = [chat(7, 'api-refactor')]
    const { togglePin, rows } = useTerminalPinnedChats()

    togglePin(7)

    expect(rows.value.map((row) => row.slug)).toEqual(['agentws-7'])
  })

  it('keeps pin order rather than the listing’s', () => {
    const { recents } = useAgentSessionsAll()
    recents.value = [chat(1, 'first'), chat(2, 'second'), chat(3, 'third')]
    const { togglePin, rows } = useTerminalPinnedChats()

    togglePin(3)
    togglePin(1)

    expect(rows.value.map((row) => row.name)).toEqual(['third', 'first'])
  })

  it('unpins by slug, which is what the Code view’s rows are keyed on', () => {
    const { recents } = useAgentSessionsAll()
    recents.value = [chat(7, 'api-refactor')]
    const { togglePin, unpinSlug, isPinned } = useTerminalPinnedChats()
    togglePin(7)

    unpinSlug('agentws-7')

    expect(isPinned(7)).toBe(false)
  })

  it('drops a pin whose chat the listing no longer carries', async () => {
    const { recents, recentsLoaded } = useAgentSessionsAll()
    recents.value = [chat(7, 'api-refactor'), chat(8, 'docs-pass')]
    recentsLoaded.value = true
    const { togglePin, pinnedIds } = useTerminalPinnedChats()
    togglePin(7)
    togglePin(8)
    await nextTick()

    recents.value = [chat(8, 'docs-pass')]
    await nextTick()

    expect(pinnedIds.value).toEqual([8])
  })

  // The pin set outlives a run; the listing does not. Pruning against a list
  // that has not loaded — or that is empty because the Agents area is gated
  // off — would silently discard every pin the user made.
  it('does not prune before the listing has loaded', async () => {
    const { recents, recentsLoaded } = useAgentSessionsAll()
    recents.value = [chat(7, 'api-refactor')]
    recentsLoaded.value = true
    const { togglePin, pinnedIds } = useTerminalPinnedChats()
    togglePin(7)
    await nextTick()

    recentsLoaded.value = false
    recents.value = []
    await nextTick()

    expect(pinnedIds.value).toEqual([7])
  })
})
