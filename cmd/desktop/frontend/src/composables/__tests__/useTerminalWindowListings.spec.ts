import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetTerminalWindowListingsForTests, useTerminalWindowListings } from '../useTerminalWindowListings'
import type { TerminalClient } from '../../lib/terminalClient'
import type { TerminalSessionRow } from '../useTerminalSessions'

function row(slug: string): TerminalSessionRow {
  return { id: slug, name: slug, slug, repo: 'owner/repo', state: 'active' }
}

// One deferred listing per sweep, so a sweep can be held open while more
// triggers arrive.
function heldClient() {
  const releases: (() => void)[] = []
  const listWindows = vi.fn((slugs: string[]) => new Promise((resolve) => {
    releases.push(() => resolve(Object.fromEntries(slugs.map((slug) =>
      [slug, [{ windowId: `@${slug}`, name: slug, active: true, width: 80, height: 24 }]]))))
  }))
  return {
    listWindows,
    releaseAll: () => {
      const pending = releases.splice(0)
      for (const release of pending) release()
    },
    client: { listWindows } as unknown as TerminalClient,
  }
}

describe('useTerminalWindowListings', () => {
  beforeEach(() => resetTerminalWindowListingsForTests())

  // The whole sidebar is one round trip: per slug, tmux answered an unattached
  // session by spawning twice, so a sweep cost two processes per row.
  it('asks for every session in one call', async () => {
    const { listWindows, releaseAll, client } = heldClient()
    const { refresh } = useTerminalWindowListings()

    const sweep = refresh(client, [row('one'), row('two')])
    expect(listWindows).toHaveBeenCalledTimes(1)
    expect(listWindows).toHaveBeenCalledWith(['one', 'two'])

    releaseAll()
    await sweep
  })

  // A switch moves the route, the pool and the session list within a few
  // ticks. Sweeping per trigger queued the attach behind all of them, so the
  // burst has to collapse to one follow-up.
  it('collapses triggers arriving during a sweep into a single follow-up', async () => {
    const { listWindows, releaseAll, client } = heldClient()
    const { refresh } = useTerminalWindowListings()
    const rows = [row('one'), row('two')]

    void refresh(client, rows)
    expect(listWindows).toHaveBeenCalledTimes(1)

    // Three more triggers while the first sweep is still open.
    void refresh(client, rows)
    void refresh(client, rows)
    void refresh(client, rows)
    expect(listWindows).toHaveBeenCalledTimes(1)

    releaseAll()
    await flushPromises()

    // One follow-up sweep, not three.
    expect(listWindows).toHaveBeenCalledTimes(2)
  })

  it('publishes the sweep and lets a later one run', async () => {
    const { releaseAll, client, listWindows } = heldClient()
    const { listings, settled, refresh } = useTerminalWindowListings()
    expect(settled.value).toBe(false)

    const first = refresh(client, [row('one')])
    releaseAll()
    await first

    expect(listings.value.one.map((window) => window.windowId)).toEqual(['@one'])
    // The tree holds its first paint until this flips, so a sweep that answered
    // has to say so.
    expect(settled.value).toBe(true)

    const second = refresh(client, [row('two')])
    releaseAll()
    await second

    expect(listings.value).not.toHaveProperty('one')
    expect(listWindows).toHaveBeenCalledTimes(2)
  })

  // Otherwise the tree waits on a sweep that already failed and never paints.
  it('settles even when the sweep fails, keeping the last-known listings', async () => {
    const listWindows = vi.fn(async () => { throw new Error('tmux is gone') })
    const client = { listWindows } as unknown as TerminalClient
    const { listings, settled, refresh } = useTerminalWindowListings()

    await refresh(client, [row('one')])

    expect(settled.value).toBe(true)
    expect(listings.value).toEqual({})
  })
})
