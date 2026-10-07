import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ ItemSessions: vi.fn(), ItemChats: vi.fn(), On: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  ItemSessions: mocks.ItemSessions,
  ItemChats: mocks.ItemChats,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

import { useItemSessions } from '../useItemSessions'

const session = (id: string) => ({
  id,
  name: id,
  slug: id,
  repo: 'acme/site',
  state: 'active',
  running: false,
  createdAt: new Date(0).toISOString(),
})

beforeEach(() => {
  vi.clearAllMocks()
  mocks.On.mockReturnValue(() => {})
  mocks.ItemChats.mockResolvedValue([])
})

describe('useItemSessions', () => {
  it('loads the selected item’s sessions', async () => {
    mocks.ItemSessions.mockResolvedValue([session('s1')])
    const s = useItemSessions()
    await s.load(7)
    expect(mocks.ItemSessions).toHaveBeenCalledWith(7)
    expect(s.sessions.value).toHaveLength(1)
  })

  it('clears the list when nothing is selected, without asking the backend', async () => {
    mocks.ItemSessions.mockResolvedValue([session('s1')])
    const s = useItemSessions()
    await s.load(7)
    await s.load(null)
    expect(s.sessions.value).toEqual([])
    expect(mocks.ItemSessions).toHaveBeenCalledTimes(1)
  })

  // An install with no hive behind it answers with an error; that means "no
  // sessions to show", not a pane full of red.
  it('shows nothing when the backend cannot answer', async () => {
    mocks.ItemSessions.mockRejectedValue(new Error('session links are unavailable'))
    const s = useItemSessions()
    await s.load(7)
    expect(s.sessions.value).toEqual([])
  })

  it('reloads the item currently on screen when a job settles', async () => {
    mocks.ItemSessions.mockResolvedValue([session('s1')])
    const s = useItemSessions()
    await s.load(7)
    mocks.ItemSessions.mockResolvedValue([session('s1'), session('s2')])
    ;(mocks.On.mock.calls[0] as [string, () => void])[1]()
    await flushPromises()
    expect(mocks.On.mock.calls[0]?.[0]).toBe('jobs:updated')
    expect(mocks.ItemSessions).toHaveBeenLastCalledWith(7)
    expect(s.sessions.value).toHaveLength(2)
  })

  // A slow answer for a previously selected item must never paint over the
  // item the user is actually looking at.
  it('drops an answer for an item that is no longer selected', async () => {
    let releaseSlow: (value: unknown) => void = () => {}
    mocks.ItemSessions.mockReturnValueOnce(
      new Promise((resolve) => {
        releaseSlow = resolve
      }),
    )
    const s = useItemSessions()
    const slow = s.load(7)

    mocks.ItemSessions.mockResolvedValueOnce([session('current')])
    await s.load(8)

    releaseSlow([session('stale')])
    await slow
    expect(s.sessions.value.map((entry) => entry.id)).toEqual(['current'])
  })

  it('loads the item’s chats alongside its sessions, each failing on its own', async () => {
    mocks.ItemSessions.mockRejectedValue(new Error('hive.db locked'))
    mocks.ItemChats.mockResolvedValue([
      { id: '9', workspace: 'triage', name: 'chat', createdAt: new Date(0).toISOString() },
    ])
    const s = useItemSessions()
    await s.load(7)
    expect(mocks.ItemChats).toHaveBeenCalledWith(7)
    expect(s.sessions.value).toEqual([])
    expect(s.chats.value.map((chat) => chat.id)).toEqual(['9'])

    await s.load(null)
    expect(s.chats.value).toEqual([])
  })
})
