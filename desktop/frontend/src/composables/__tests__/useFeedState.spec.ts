import { describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { useFeedState } from '../useFeedState'
import type { FeedItem, Profile } from '../../types/feed'

const mocks = vi.hoisted(() => ({
  ActionsFor: vi.fn(),
  Hide: vi.fn(),
  Items: vi.fn(),
  Profiles: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/feedservice', () => ({
  ActionsFor: mocks.ActionsFor,
  Items: mocks.Items,
  Profiles: mocks.Profiles,
}))

vi.mock('@wailsio/runtime', () => ({
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
    mocks.Items.mockImplementation((profileID: string, feedID: string) => Promise.resolve(itemSets[`${profileID}:${feedID}`] ?? []))
    mocks.ActionsFor.mockImplementation((kind: string) => Promise.resolve([{ id: `action-${kind}`, title: kind }]))
    mocks.Hide.mockResolvedValue(undefined)
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

    await state.selectSidebar({ type: 'unread' })
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
})
