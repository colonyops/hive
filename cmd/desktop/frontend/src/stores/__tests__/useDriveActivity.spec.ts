import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDriveActivity } from '../useDriveActivity'

const events = vi.hoisted(() => ({
  handlers: new Map<string, (event: { data: unknown }) => void>(),
}))
vi.mock('../../composables/useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: (event: { data: unknown }) => void) => {
    events.handlers.set(name, handler)
  },
}))

function wrote(author: { session?: string; hiveSession?: string }, paths = ['plans/x.md']): void {
  events.handlers.get('drive:updated')?.({ data: { space: 'workspaces/web-app', paths, ...author } })
}

describe('useDriveActivity', () => {
  beforeEach(() => {
    events.handlers.clear()
  })

  it('marks a write unseen per author, chat or hive session', () => {
    const activity = useDriveActivity()
    wrote({ session: '7' })
    wrote({ hiveSession: 'abc123' })

    expect(activity.isUnseen('7')).toBe(true)
    expect(activity.isUnseen('abc123')).toBe(true)
    expect(activity.isUnseen('9')).toBe(false)
    expect(activity.isUnseen(null)).toBe(false)
  })

  it('leaves a change the watcher found on disk, with no author, unmarked', () => {
    const activity = useDriveActivity()
    wrote({ session: '', hiveSession: '' })
    expect(activity.unseen.value.size).toBe(0)
  })

  it('treats a write by an author a pane shows as seen, and clears the dot when a pane opens', () => {
    const activity = useDriveActivity()
    wrote({ session: '7' })
    activity.registerVisible('7')
    expect(activity.isUnseen('7')).toBe(false)

    wrote({ session: '7' })
    expect(activity.isUnseen('7')).toBe(false)

    activity.unregisterVisible('7')
    wrote({ session: '7' })
    expect(activity.isUnseen('7')).toBe(true)
  })

  // A chat pinned in Code is the same author as in Chats, so two panes can
  // show it while one mounts and the other unmounts.
  it('keeps an author visible until every pane showing it unregisters', () => {
    const activity = useDriveActivity()
    activity.registerVisible('7')
    activity.registerVisible('7')
    activity.unregisterVisible('7')
    wrote({ session: '7' })
    expect(activity.isUnseen('7')).toBe(false)

    activity.unregisterVisible('7')
    wrote({ session: '7' })
    expect(activity.isUnseen('7')).toBe(true)
  })

  it("remembers each author's last write per space, and which documents nobody opened", () => {
    const activity = useDriveActivity()
    wrote({ session: '7' }, ['a.md', 'b.md'])

    expect(activity.lastWrite('7', 'workspaces/web-app')).toBe('b.md')
    expect(activity.lastWrite('7', 'acme/site')).toBeNull()
    expect(activity.isUnopened('workspaces/web-app', 'a.md')).toBe(true)

    activity.markOpened('workspaces/web-app', 'a.md')
    expect(activity.isUnopened('workspaces/web-app', 'a.md')).toBe(false)
  })
})
