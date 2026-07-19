import { describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { useFeedState } from '../useFeedState'

const mocks = vi.hoisted(() => ({
  ListFlows: vi.fn(),
  GetFlow: vi.fn(),
  CreateFlow: vi.fn(),
  DeleteFlow: vi.fn(),
  FeedItems: vi.fn(),
  FeedItemCounts: vi.fn(),
  MarkFeedItemRead: vi.fn(),
  ActionsFor: vi.fn(),
  On: vi.fn(),
  Hide: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/flowsservice', () => ({
  ListFlows: mocks.ListFlows,
  GetFlow: mocks.GetFlow,
  CreateFlow: mocks.CreateFlow,
  DeleteFlow: mocks.DeleteFlow,
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/pipelineservice', () => ({
  FeedItems: mocks.FeedItems,
  FeedItemCounts: mocks.FeedItemCounts,
  MarkFeedItemRead: mocks.MarkFeedItemRead,
  ActionsFor: mocks.ActionsFor,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Window: { Hide: mocks.Hide },
}))

const flowSummary = { id: 'triage', name: 'Frontend Triage', enabled: true, valid: true }
const flow = {
  id: 'triage',
  name: 'Frontend Triage',
  enabled: true,
  nodes: [
    { id: 'prs', type: 'github-source' },
    { id: 'my-prs', type: 'feed', name: 'My PRs' },
  ],
  wires: [],
}

// Mounts a trivial component so the composable's onMounted runs.
function mountState() {
  let state!: ReturnType<typeof useFeedState>
  mount({ setup() { state = useFeedState(); return () => null } })
  return () => state
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.ListFlows.mockResolvedValue([flowSummary])
  mocks.GetFlow.mockResolvedValue(flow)
  mocks.FeedItemCounts.mockResolvedValue([{ feedId: 'triage/my-prs', total: 3, unread: 2 }])
  mocks.FeedItems.mockResolvedValue([])
  mocks.ActionsFor.mockResolvedValue([])
  mocks.On.mockReturnValue(() => {})
})

describe('useFeedState', () => {
  it('loads profiles from flows and populates feeds from the flow feed nodes', async () => {
    const get = mountState()
    await flushPromises()

    const state = get()
    expect(state.profiles.value).toHaveLength(1)
    expect(state.activeProfileId.value).toBe('triage')
    expect(state.activeProfile.value?.feeds).toEqual([
      { id: 'triage/my-prs', name: 'My PRs', count: 3, newCount: 2 },
    ])
    expect(state.activeProfile.value?.totalCount).toBe(3)
    expect(state.activeProfile.value?.unreadCount).toBe(2)
    expect(state.activeProfile.value?.sourceSummary).toBe('GitHub · 1 source')
  })

  it('loads a feed\'s items from feed_item, decoding the payload', async () => {
    mocks.FeedItems.mockImplementation((feedID: string) =>
      Promise.resolve(
        feedID === 'triage/my-prs'
          ? [{ feedId: 'triage/my-prs', itemId: 'o/r#1', unread: true, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR', repo: 'o/r', num: 1 } }]
          : [],
      ),
    )
    const get = mountState()
    await flushPromises()

    await get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' })
    await flushPromises()

    const items = get().items.value
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ id: 'o/r#1', feedId: 'triage/my-prs', title: 'Fix it', kind: 'PR' })
  })

  it('creates a profile by creating a flow', async () => {
    mocks.CreateFlow.mockResolvedValue({ id: 'new', name: 'New', enabled: true, valid: true })
    const get = mountState()
    await flushPromises()

    await get().createProfile('New')
    await flushPromises()

    expect(mocks.CreateFlow).toHaveBeenCalledWith('New')
    expect(get().profiles.value.some((p) => p.id === 'new')).toBe(true)
  })

  it('deletes a profile by deleting its flow', async () => {
    mocks.DeleteFlow.mockResolvedValue(undefined)
    const get = mountState()
    await flushPromises()

    await get().deleteProfile('triage')
    await flushPromises()

    expect(mocks.DeleteFlow).toHaveBeenCalledWith('triage')
  })

  it('marks an item read via the pipeline service', async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: true, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR' } },
    ])
    mocks.MarkFeedItemRead.mockResolvedValue(undefined)
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'all' })
    await flushPromises()

    await get().selectItem('o/r#1')
    await flushPromises()

    expect(mocks.MarkFeedItemRead).toHaveBeenCalledWith('triage/my-prs', 'o/r#1')
  })
})
