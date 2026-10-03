import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { TerminalSessionRow } from '../../../stores/useTerminalSessions'
import type { TerminalModeContext } from '../terminalModeContext'
import { attachedSessionCommands } from '../useTerminalCommands'

const hive: TerminalSessionRow = { id: '1', name: 'parser', slug: 'parser', repo: 'r', state: 'active' }
const scratch: TerminalSessionRow = { id: 's', name: 'Scratch', slug: 'scratch', repo: '', state: 'active' }
const chat: TerminalSessionRow = { id: 'c', name: 'chat', slug: 'agentws-1', repo: '', state: 'active' }

function context(running: boolean): TerminalModeContext {
  return {
    tree: {
      isHiveSession: (row: TerminalSessionRow) => row === hive,
      isScratch: (row: TerminalSessionRow) => row === scratch,
      isChat: (row: TerminalSessionRow) => row === chat,
      rowRunning: () => running,
      windowRowsFor: () => [{ windowId: '@1', name: 'agent', active: true, live: true, indicator: null }],
      unpinSlug: vi.fn(),
    },
    ops: {
      agentWindowBusy: ref(false),
      agentProfiles: ref(['claude']),
      sessionActionEntries: ref([{ kind: 'action', id: 'open-pr', label: 'Open PR' }]),
      windowActionEntries: ref([{ kind: 'separator' }, { kind: 'action', id: 'copy', label: 'Copy' }]),
      runWindowAction: vi.fn(),
    },
    attach: { startSession: vi.fn() },
    sessions: {},
  } as unknown as TerminalModeContext
}

const ids = (row: TerminalSessionRow, running: boolean) =>
  attachedSessionCommands(context(running), row).map((command) => command.id)

describe('attachedSessionCommands', () => {
  it("lists a running hive session's operations, its configured actions, and the on-screen window's", () => {
    expect(ids(hive, true)).toEqual([
      'terminal:session:kill',
      'terminal:session:agent:claude',
      'terminal:session:detail',
      'terminal:session:rename',
      'terminal:session:recycle',
      'terminal:session:delete',
      'terminal:session:open-pr',
      'terminal:window:action:copy',
    ])
  })

  it('offers a start and no agents while nothing runs', () => {
    expect(ids(hive, false)).toContain('terminal:session:start')
    expect(ids(hive, false)).not.toContain('terminal:session:agent:claude')
  })

  it('gives the scratch terminal its terminal operations only', () => {
    expect(ids(scratch, false)).toEqual(['terminal:session:start', 'terminal:session:kill'])
  })

  it("gives a pinned chat the pin's two operations, grouped under its name", () => {
    const commands = attachedSessionCommands(context(true), chat)
    expect(commands.map((command) => [command.id, command.scope, command.group])).toEqual([
      ['terminal:chat:open-in-agents', 'goto', 'chat'],
      ['terminal:chat:unpin', 'actions', 'chat'],
    ])
  })

  it("names the on-screen window in a window action's hint", () => {
    const copy = attachedSessionCommands(context(true), hive).find((command) => command.id.endsWith(':copy'))
    expect(copy?.hint).toBe('agent')
  })
})
