import { describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { useFeedState } from '../useFeedState'
import type { FeedItem, Profile } from '../../types/feed'

const mocks = vi.hoisted(() => ({
  ActionsFor: vi.fn(),
  CreateProfile: vi.fn(),
  Hide: vi.fn(),
  Items: vi.fn(),
  MarkRead: vi.fn(),
  On: vi.fn(),
  Profiles: vi.fn(),
  Refresh: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/feedservice', () => ({
  ActionsFor: mocks.ActionsFor,
  CreateProfile: mocks.CreateProfile,
  Items: mocks.Items,
  MarkRead: mocks.MarkRead,
  Profiles: mocks.Profiles,
  Refresh: mocks.Refresh,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: mocks.On,
  },
  Window: {
    Hide: mocks.Hide,
  },
}))

const profiles: Profile[] = [
  {
    id: 'personal',
    letter: 'P',
    name: 'Personal',
    sourceSummary: '2 repos',
    totalCount: 2,
    unreadCount: 1,
    feeds: [
      { id: 'desktop', name: 'Desktop UI', count: 2, newCount: 1 },
      { id: 'backend', name: 'Backend', count: 1, newCount: 0 },
    ],
  },
  {
    id: 'work',
    letter: 'W',
    name: 'Work',
    sourceSummary: '1 repo',
    totalCount: 1,
    unreadCount: 0,
    feeds: [{ id: 'ops', name: 'Ops', count: 1, newCount: 0 }],
  },
]

function feedItem(id: string, kind: string, title: string): FeedItem {
  return {
    id,
    kind,
    repo: 'colonyops/hive',
    num: 42,
    title,
    author: 'hayden',
    age: '5m',
    unread: true,
    labels: null,
    branch: 'feat/desktop-ui-shell',
    body: 'Body',
    prompt: 'Prompt',
    url: 'https://github.com/colonyops/hive/pull/42',
  }
}

const itemSets: Record<string, FeedItem[]> = {
  'personal:': [feedItem('pr-1', 'PR', 'First PR'), feedItem('issue-1', 'Issue', 'First issue')],
  'personal:desktop': [feedItem('desktop-1', 'PR', 'Desktop PR')],
  'work:': [feedItem('work-1', 'Issue', 'Work issue')],
}

function mountState() {
  let state!: ReturnType<typeof useFeedState>
  const wrapper = mount({
    template: '<div />',
    setup() {
      state = useFeedState()
      return {}
    },
  })

  return { state, wrapper }
}

async function mountLoadedState() {
  const mounted = mountState()
  await flushPromises()
  return mounted
}

describe('useFeedState', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Profiles.mockResolvedValue(profiles)
    // Clone so markItemRead's in-place `item.unread = false` on one test never
    // leaks into the shared fixture arrays seen by later tests.
    mocks.Items.mockImplementation((profileID: string, feedID: string) =>
      Promise.resolve((itemSets[`${profileID}:${feedID}`] ?? []).map((item) => ({ ...item }))))
    mocks.ActionsFor.mockImplementation((kind: string) => Promise.resolve([{ id: `action-${kind}`, title: kind }]))
    mocks.Hide.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
    mocks.MarkRead.mockResolvedValue(undefined)
    mocks.Refresh.mockResolvedValue(false)
  })

  it('loads profiles on mount and switches profiles by resetting selection and reloading items', async () => {
    const { state, wrapper } = await mountLoadedState()

    state.selection.value = { type: 'feed', feedId: 'desktop' }
    state.unreadOnly.value = true
    mocks.Items.mockClear()

    await state.selectProfile('work')

    expect(state.activeProfileId.value).toBe('work')
    expect(state.selection.value).toEqual({ type: 'all' })
    expect(state.unreadOnly.value).toBe(false)
    expect(mocks.Items).toHaveBeenCalledWith('work', '')
    expect(state.items.value.map((item) => item.id)).toEqual(['work-1'])

    wrapper.unmount()
  })

  it('loads sidebar feed items with the active profile and updates the title', async () => {
    const { state, wrapper } = await mountLoadedState()
    mocks.Items.mockClear()

    await state.selectSidebar({ type: 'feed', feedId: 'desktop' })

    expect(mocks.Items).toHaveBeenCalledWith('personal', 'desktop')
    expect(state.title.value).toBe('Desktop UI')
    expect(state.items.value.map((item) => item.id)).toEqual(['desktop-1'])

    wrapper.unmount()
  })

  it('toggles unreadOnly without reloading items', async () => {
    const { state, wrapper } = await mountLoadedState()
    mocks.Items.mockClear()

    state.toggleUnread()
    expect(state.unreadOnly.value).toBe(true)

    state.toggleUnread()
    expect(state.unreadOnly.value).toBe(false)
    expect(mocks.Items).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('exits the sidebar Unread view when the filter is toggled off', async () => {
    const { state, wrapper } = await mountLoadedState()

    await state.selectUnreadView()
    expect(state.unreadOnly.value).toBe(true)
    expect(state.title.value).toBe('Unread')

    await state.toggleUnread()

    expect(state.unreadOnly.value).toBe(false)
    expect(state.selection.value).toEqual({ type: 'all' })
    expect(state.title.value).toBe('All items')

    wrapper.unmount()
  })

  it('moves selection to the first unread item when the filter hides the selected one', async () => {
    mocks.Items.mockResolvedValue([
      { ...feedItem('pr-1', 'PR', 'First PR'), unread: false },
      feedItem('issue-1', 'Issue', 'First issue'),
    ])
    const { state, wrapper } = await mountLoadedState()

    await state.selectItem('pr-1')
    await state.toggleUnread()

    expect(state.unreadOnly.value).toBe(true)
    expect(state.selectedId.value).toBe('issue-1')
    expect(mocks.ActionsFor).toHaveBeenCalledWith('Issue')

    wrapper.unmount()
  })

  it('clears items, selection, and actions when loading items fails', async () => {
    const { state, wrapper } = await mountLoadedState()
    expect(state.items.value).not.toHaveLength(0)
    expect(state.selectedId.value).toBe('pr-1')
    expect(state.actions.value).not.toHaveLength(0)

    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    mocks.Items.mockRejectedValue(new Error('items failed'))

    await state.selectSidebar({ type: 'all' })

    expect(state.items.value).toEqual([])
    expect(state.selectedId.value).toBeNull()
    expect(state.actions.value).toEqual([])
    expect(warn).toHaveBeenCalled()

    warn.mockRestore()
    wrapper.unmount()
  })

  it('refreshes items for the currently selected feed', async () => {
    const { state, wrapper } = await mountLoadedState()

    await state.selectSidebar({ type: 'feed', feedId: 'desktop' })
    mocks.Items.mockClear()

    await state.refresh()

    expect(mocks.Items).toHaveBeenCalledWith('personal', 'desktop')

    wrapper.unmount()
  })

  it('enters the sidebar Unread view and auto-selects the first unread item', async () => {
    // Entering the Unread view re-anchors a read (filtered-out) selection to
    // the first unread item, same as toggling the unread chip.
    mocks.Items.mockResolvedValue([
      { ...feedItem('pr-1', 'PR', 'First PR'), unread: false },
      feedItem('issue-1', 'Issue', 'First issue'),
    ])
    const { state, wrapper } = await mountLoadedState()
    expect(state.selectedId.value).toBe('pr-1')

    await state.selectUnreadView()

    expect(state.unreadOnly.value).toBe(true)
    expect(state.title.value).toBe('Unread')
    expect(state.selectedId.value).toBe('issue-1')

    wrapper.unmount()
  })

  it('refreshes counts and active items when feed:updated fires', async () => {
    let handler: ((event: { data: unknown }) => void) | undefined
    mocks.On.mockImplementation((event: string, cb: (event: { data: unknown }) => void) => {
      if (event === 'feed:updated') handler = cb
      return () => {}
    })
    const { state, wrapper } = await mountLoadedState()
    mocks.Items.mockClear()
    mocks.Profiles.mockClear()
    mocks.Profiles.mockResolvedValue([{ ...profiles[0], unreadCount: 9 }, profiles[1]])

    handler?.({ data: 'personal' })
    await flushPromises()

    expect(mocks.Profiles).toHaveBeenCalled()
    expect(state.profiles.value[0]?.unreadCount).toBe(9)
    expect(mocks.Items).toHaveBeenCalledWith('personal', '')

    mocks.Items.mockClear()
    handler?.({ data: 'work' })
    await flushPromises()
    expect(mocks.Items).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('drops stale item loads that resolve after a newer request', async () => {
    const { state, wrapper } = await mountLoadedState()

    let resolveSlow!: (items: FeedItem[]) => void
    const slow = new Promise<FeedItem[]>((resolve) => { resolveSlow = resolve })
    mocks.Items.mockReturnValueOnce(slow)

    const slowLoad = state.selectSidebar({ type: 'feed', feedId: 'desktop' })
    await state.selectSidebar({ type: 'all' })
    expect(state.items.value.map((item) => item.id)).toEqual(['pr-1', 'issue-1'])

    resolveSlow([feedItem('stale-1', 'PR', 'Stale PR')])
    await slowLoad
    await flushPromises()

    expect(state.items.value.map((item) => item.id)).toEqual(['pr-1', 'issue-1'])
    expect(state.selectedId.value).toBe('pr-1')

    wrapper.unmount()
  })

  it('classifies load failures into user-facing messages and clears them on success', async () => {
    const { state, wrapper } = await mountLoadedState()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    mocks.Items.mockRejectedValueOnce(new Error('github: rate limited'))
    await state.refresh()
    expect(state.loadError.value).toContain('rate limit')

    mocks.Items.mockRejectedValueOnce(new Error('github: unreachable: dial tcp'))
    await state.refresh()
    expect(state.loadError.value).toBe("Can't reach GitHub right now.")

    await state.refresh()
    expect(state.loadError.value).toBeNull()
    expect(state.items.value).not.toHaveLength(0)

    warn.mockRestore()
    wrapper.unmount()
  })

  it('reports profilesLoaded with no active profile when none exist', async () => {
    mocks.Profiles.mockResolvedValue([])
    const { state, wrapper } = await mountLoadedState()

    expect(state.profilesLoaded.value).toBe(true)
    expect(state.profiles.value).toEqual([])
    expect(state.activeProfileId.value).toBe('')

    wrapper.unmount()
  })

  it('creates a profile and selects it', async () => {
    mocks.Profiles.mockResolvedValue([])
    mocks.CreateProfile.mockImplementation((name: string) => Promise.resolve({
      ...profiles[0],
      id: 'created-1',
      name,
    }))
    const { state, wrapper } = await mountLoadedState()

    await state.createProfile('My Triage')

    expect(mocks.CreateProfile).toHaveBeenCalledWith('My Triage')
    expect(state.profiles.value.map((profile) => profile.id)).toEqual(['created-1'])
    expect(state.activeProfileId.value).toBe('created-1')
    expect(state.createProfileError.value).toBeNull()

    wrapper.unmount()
  })

  it('surfaces profile creation failures', async () => {
    mocks.Profiles.mockResolvedValue([])
    mocks.CreateProfile.mockRejectedValue(new Error('boom'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { state, wrapper } = await mountLoadedState()

    await state.createProfile('My Triage')

    expect(state.createProfileError.value).toContain('boom')
    expect(state.creatingProfile.value).toBe(false)

    warn.mockRestore()
    wrapper.unmount()
  })

  it('loads actions for the selected item kind', async () => {
    const { state, wrapper } = await mountLoadedState()
    mocks.ActionsFor.mockClear()

    await state.selectItem('issue-1')

    expect(state.selectedId.value).toBe('issue-1')
    expect(mocks.ActionsFor).toHaveBeenCalledWith('Issue')
    expect(state.actions.value).toEqual([{ id: 'action-Issue', title: 'Issue' }])

    wrapper.unmount()
  })

  it('hides the Wails window through the runtime binding', async () => {
    const { state, wrapper } = await mountLoadedState()

    await state.hideWindow()

    expect(mocks.Hide).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  it('keeps the selected item when a reload still contains it', async () => {
    let handler: ((event: { data: unknown }) => void) | undefined
    mocks.On.mockImplementation((event: string, cb: (event: { data: unknown }) => void) => {
      if (event === 'feed:updated') handler = cb
      return () => {}
    })
    const { state, wrapper } = await mountLoadedState()
    expect(state.selectedId.value).toBe('pr-1')

    await state.selectItem('issue-1')
    mocks.Items.mockClear()

    handler?.({ data: 'personal' })
    await flushPromises()

    expect(state.selectedId.value).toBe('issue-1')
    expect(mocks.Items).toHaveBeenCalled()

    wrapper.unmount()
  })

  it('clears the unread filter when navigating the sidebar', async () => {
    const { state, wrapper } = await mountLoadedState()

    await state.selectUnreadView()
    expect(state.unreadOnly.value).toBe(true)

    mocks.Items.mockClear()
    await state.selectSidebar({ type: 'all' })

    expect(state.unreadOnly.value).toBe(false)
    expect(mocks.Items).toHaveBeenCalled()
    expect(state.title.value).toBe('All items')

    wrapper.unmount()
  })

  it('marks an unread item read on selection', async () => {
    const { state, wrapper } = await mountLoadedState()

    await state.selectItem('issue-1')

    expect(mocks.MarkRead).toHaveBeenCalledWith('personal', 'issue-1')
    expect(state.items.value.find((item) => item.id === 'issue-1')?.unread).toBe(false)

    mocks.MarkRead.mockClear()
    await state.selectItem('issue-1')

    expect(mocks.MarkRead).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('keeps onboarding gated off when profiles fail to load', async () => {
    mocks.Profiles.mockRejectedValueOnce(new Error('boom'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { state, wrapper } = await mountLoadedState()

    expect(state.profilesLoaded.value).toBe(false)
    expect(state.profilesError.value).toBe('Could not load your workspaces.')

    mocks.Profiles.mockResolvedValue(profiles)
    await state.loadProfiles()

    expect(state.profilesError.value).toBeNull()
    expect(state.profilesLoaded.value).toBe(true)

    warn.mockRestore()
    wrapper.unmount()
  })

  it('manual refresh bypasses the cache via Refresh', async () => {
    const { state, wrapper } = await mountLoadedState()
    mocks.Items.mockClear()

    await state.refresh()

    expect(mocks.Refresh).toHaveBeenCalledWith('personal')
    expect(mocks.Items).toHaveBeenCalledWith('personal', '')

    mocks.Items.mockClear()
    mocks.Refresh.mockRejectedValueOnce(new Error('github: rate limited'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    await state.refresh()

    expect(state.loadError.value).toContain('rate limit')
    expect(mocks.Items).not.toHaveBeenCalled()

    warn.mockRestore()
    wrapper.unmount()
  })

  it('ignores re-entrant createProfile calls', async () => {
    mocks.Profiles.mockResolvedValue([])
    let resolveCreate!: (profile: Profile) => void
    mocks.CreateProfile.mockReturnValue(new Promise<Profile>((resolve) => { resolveCreate = resolve }))
    const { state, wrapper } = await mountLoadedState()

    const first = state.createProfile('My Triage')
    const second = state.createProfile('My Triage')

    resolveCreate({ ...profiles[0], id: 'created-1', name: 'My Triage' })
    await first
    await second

    expect(mocks.CreateProfile).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })
})
