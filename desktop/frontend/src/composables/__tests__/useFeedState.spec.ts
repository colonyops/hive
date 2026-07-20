import { afterEach, describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { useFeedState } from '../useFeedState'

const mocks = vi.hoisted(() => ({
  ListFlows: vi.fn(),
  GetFlow: vi.fn(),
  CreateFlow: vi.fn(),
  DeleteFlow: vi.fn(),
  GetSidebar: vi.fn(),
  SaveSidebar: vi.fn(),
  FeedItems: vi.fn(),
  FeedItemCounts: vi.fn(),
  MarkFeedItemRead: vi.fn(),
  ActionViews: vi.fn(),
  ActionRun: vi.fn(),
  InvokeAction: vi.fn(),
  SessionLaunchOptions: vi.fn(),
  On: vi.fn(),
  Hide: vi.fn(),
  OpenURL: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/flowsservice', () => ({
  ListFlows: mocks.ListFlows,
  GetFlow: mocks.GetFlow,
  CreateFlow: mocks.CreateFlow,
  DeleteFlow: mocks.DeleteFlow,
  GetSidebar: mocks.GetSidebar,
  SaveSidebar: mocks.SaveSidebar,
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/pipelineservice', () => ({
  FeedItems: mocks.FeedItems,
  FeedItemCounts: mocks.FeedItemCounts,
  MarkFeedItemRead: mocks.MarkFeedItemRead,
  ActionViews: mocks.ActionViews,
  ActionRun: mocks.ActionRun,
  InvokeAction: mocks.InvokeAction,
  SessionLaunchOptions: mocks.SessionLaunchOptions,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Window: { Hide: mocks.Hide },
  Browser: { OpenURL: mocks.OpenURL },
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

afterEach(() => vi.unstubAllGlobals())

beforeEach(() => {
  vi.clearAllMocks()
  mocks.ListFlows.mockResolvedValue([flowSummary])
  mocks.GetFlow.mockResolvedValue(flow)
  mocks.GetSidebar.mockResolvedValue({ items: [] })
  mocks.SaveSidebar.mockResolvedValue(undefined)
  mocks.FeedItemCounts.mockResolvedValue([{ feedId: 'triage/my-prs', total: 3, unread: 2 }])
  mocks.FeedItems.mockResolvedValue([])
  mocks.ActionViews.mockResolvedValue([])
  mocks.ActionRun.mockResolvedValue({ commandId: 1, status: 'done' })
  localStorage.clear()
  mocks.InvokeAction.mockResolvedValue({ commandId: 1, status: 'done' })
  mocks.SessionLaunchOptions.mockResolvedValue({ repositories: [{ name: 'hive', repository: 'https://github.com/colonyops/hive.git' }], defaultRepository: 'https://github.com/colonyops/hive.git', agents: ['claude'], defaultAgent: 'claude' })
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

  it('builds a flat sidebar tree from feeds when there is no saved layout', async () => {
    const get = mountState()
    await flushPromises()
    expect(get().activeProfile.value?.tree).toEqual([
      { kind: 'feed', feed: { id: 'triage/my-prs', name: 'My PRs', count: 3, newCount: 2 } },
    ])
  })

  it('reconciles the saved sidebar layout (folders, node-id keyed) into the tree', async () => {
    mocks.GetSidebar.mockResolvedValue({
      items: [{ folder: { id: 'work', name: 'Work', feeds: ['my-prs'] } }],
    })
    const get = mountState()
    await flushPromises()

    expect(get().activeProfile.value?.tree).toEqual([
      {
        kind: 'folder',
        folder: { id: 'work', name: 'Work', feeds: [{ id: 'triage/my-prs', name: 'My PRs', count: 3, newCount: 2 }] },
      },
    ])
  })

  it('persists a reordered tree via SaveSidebar, keyed by feed node id', async () => {
    mocks.SaveSidebar.mockResolvedValue(undefined)
    const get = mountState()
    await flushPromises()

    const feed = { id: 'triage/my-prs', name: 'My PRs', count: 3, newCount: 2 }
    await get().reorderFeeds('triage', [
      { kind: 'folder', folder: { id: 'work', name: 'Work', feeds: [feed] } },
    ])

    expect(mocks.SaveSidebar).toHaveBeenCalledWith('triage', {
      items: [{ folder: { id: 'work', name: 'Work', feeds: ['my-prs'] } }],
    })
    // The in-memory tree updates optimistically.
    expect(get().activeProfile.value?.tree?.[0]).toMatchObject({ kind: 'folder', folder: { id: 'work' } })
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

  it('opens the selected item URL in the browser', async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR', url: 'https://github.com/o/r/pull/1' } },
    ])
    mocks.OpenURL.mockResolvedValue(undefined)
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'all' })
    await flushPromises()

    await get().openSelectedInBrowser()

    expect(mocks.OpenURL).toHaveBeenCalledWith('https://github.com/o/r/pull/1')
  })

  it('toasts instead of opening when the selected item has no URL', async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR' } },
    ])
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'all' })
    await flushPromises()

    await get().openSelectedInBrowser()

    expect(mocks.OpenURL).not.toHaveBeenCalled()
    expect(get().toasts.value.some((t) => t.severity === 'error')).toBe(true)
  })

  it('opens an arbitrary URL via openUrl', async () => {
    mocks.OpenURL.mockResolvedValue(undefined)
    const get = mountState()
    await flushPromises()

    await get().openUrl('https://example.com')

    expect(mocks.OpenURL).toHaveBeenCalledWith('https://example.com')
  })

  it('invokes the selected configured action with the selected item', async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR', labels: [] } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'launch-session' }])
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'all' })
    await flushPromises()

    await get().invokeAction('review')

    expect(mocks.InvokeAction).toHaveBeenCalledWith('review', expect.objectContaining({ id: 'o/r#1', kind: 'PR' }), {})
    expect(get().toasts.value.some((toast) => toast.message === 'Review completed')).toBe(true)
  })

  it('names published message topic and sender in success feedback', async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'notify', label: 'Notify', type: 'publish-message' }])
    mocks.InvokeAction.mockResolvedValue({ commandId: 18, status: 'done', result: { message: { topic: 'agent.session.inbox', sender: 'hive-desktop' } } })
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'all' })
    await flushPromises()

    await get().invokeAction('notify')

    expect(get().toasts.value.some((toast) => toast.message === 'Published message to agent.session.inbox as hive-desktop')).toBe(true)
  })

  it('treats a non-done action run as a failure and keeps diagnostics',async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.InvokeAction.mockResolvedValue({ commandId: 17, status: 'failed', error: 'command exited 1', stdout: 'partial', stderr: 'bad input' })
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'all' })
    await flushPromises()

    await get().invokeAction('review')

    expect(get().actionRuns.value.review).toMatchObject({ commandId: 17, status: 'failed', stderr: 'bad input' })
    expect(get().toasts.value.some((toast) => toast.severity === 'error' && toast.message === 'command exited 1')).toBe(true)
  })

  it('opens interactive launch actions before invoking and submits dialog input', async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'Fix it', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'launch-session', requiresSessionInput: true }])
    mocks.InvokeAction.mockResolvedValue({ commandId: 1, status: 'done', result: { session: { id: 'session-1', name: 'review-pr-1' } } })
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'all' })
    await flushPromises()

    await get().invokeAction('review')
    expect(mocks.SessionLaunchOptions).toHaveBeenCalledOnce()
    expect(mocks.InvokeAction).not.toHaveBeenCalled()
    expect(get().sessionLaunchAction.value?.id).toBe('review')

    await get().submitSessionLaunch({ name: 'review-pr-1', repository: 'https://github.com/colonyops/hive.git', agent: 'claude' })
    expect(mocks.InvokeAction).toHaveBeenCalledWith('review', expect.anything(), { session: { name: 'review-pr-1', repository: 'https://github.com/colonyops/hive.git', agent: 'claude' } })
    expect(get().toasts.value.some((toast) => toast.message === 'Created session review-pr-1 (session-1)')).toBe(true)
    expect(get().sessionLaunchAction.value).toBeNull()
  })

  it('scopes runs by item and rehydrates the persisted command when returning to an item', async () => {
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
      { feedId: 'triage/my-prs', itemId: 'o/r#2', unread: false, payload: { id: 'o/r#2', title: 'Two', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.InvokeAction.mockResolvedValue({ commandId: 17, status: 'failed', error: 'bad', stderr: 'details' })
    mocks.ActionRun.mockResolvedValue({ commandId: 17, status: 'failed', error: 'bad', stderr: 'details' })
    const get = mountState(); await flushPromises(); await get().selectSidebar({ type: 'all' }); await flushPromises()
    await get().invokeAction('review')
    expect(get().actionRuns.value.review?.commandId).toBe(17)
    await get().selectItem('o/r#2'); await flushPromises()
    expect(get().actionRuns.value.review).toBeUndefined()
    await get().selectItem('o/r#1'); await flushPromises()
    expect(mocks.ActionRun).toHaveBeenCalledWith(17)
    expect(get().actionRuns.value.review?.stderr).toBe('details')
  })

  it.each([null, [], { 'o/r#1': { review: 0 } }, { 'o/r#1': { review: -1 } }, { 'o/r#1': { review: '17' } }, { 'o/r#1': [] }, { '': { review: 1 } }])('rejects malformed persisted action run IDs: %j', async (stored) => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify(stored))
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    const get = mountState(); await flushPromises(); await get().selectSidebar({ type: 'all' }); await flushPromises()
    expect(mocks.ActionRun).not.toHaveBeenCalled()
  })

  it('ignores malformed persisted action run entries while rehydrating valid entries', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({
      'o/r#1': { review: 17, invalid: 0, stringy: '18', empty: 19 },
      'o/r#2': [],
      '': { review: 41 },
    }))
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review', type: 'shell' },
      { id: 'invalid', label: 'Invalid', type: 'shell' },
      { id: 'stringy', label: 'Stringy', type: 'shell' },
      { id: '', label: 'Empty', type: 'shell' },
    ])
    mocks.ActionRun.mockResolvedValue({ commandId: 17, status: 'failed', stderr: 'details' })
    const get = mountState(); await flushPromises()
    expect(get().actionRuns.value.review?.commandId).toBe(17)
    expect(mocks.ActionRun).toHaveBeenCalledTimes(1)
    expect(mocks.ActionRun).toHaveBeenCalledWith(17)
  })

  it('keeps successful action feedback when action run restoration fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.stubGlobal('localStorage', { getItem: () => { throw new Error('security denied') }, setItem: vi.fn(), clear: vi.fn(), key: vi.fn(), removeItem: vi.fn(), length: 0 })
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.InvokeAction.mockResolvedValue({ commandId: 17, status: 'done' })
    const get = mountState(); await flushPromises(); await get().selectSidebar({ type: 'all' }); await flushPromises()
    await get().invokeAction('review')
    expect(get().actionRuns.value.review?.commandId).toBe(17)
    expect(get().toasts.value.some((toast) => toast.message === 'Review completed')).toBe(true)
    expect(warn).toHaveBeenCalledWith('Unable to restore action run IDs', expect.any(Error))
    warn.mockRestore()
  })

  it('keeps successful action feedback when action run persistence fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => { throw new Error('quota exceeded') }, clear: vi.fn(), key: vi.fn(), removeItem: vi.fn(), length: 0 })
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.InvokeAction.mockResolvedValue({ commandId: 17, status: 'done' })
    const get = mountState(); await flushPromises(); await get().selectSidebar({ type: 'all' }); await flushPromises()
    await get().invokeAction('review')
    expect(get().actionRuns.value.review?.commandId).toBe(17)
    expect(get().toasts.value.some((toast) => toast.message === 'Review completed')).toBe(true)
    expect(warn).toHaveBeenCalledWith('Unable to persist action run IDs', expect.any(Error))
    warn.mockRestore()
  })

  it('drops a stale persisted command mapping without leaking it to another item', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ 'o/r#1': { review: 41 } }))
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
      { feedId: 'triage/my-prs', itemId: 'o/r#2', unread: false, payload: { id: 'o/r#2', title: 'Two', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.ActionRun.mockRejectedValue(new Error('command not found'))
    const get = mountState(); await flushPromises(); await get().selectSidebar({ type: 'all' }); await flushPromises()
    expect(mocks.ActionRun).toHaveBeenCalledWith(41)
    expect(localStorage.getItem('hive.action-run-ids')).toBe('{}')
    await get().selectItem('o/r#2'); await flushPromises()
    expect(get().actionRuns.value.review).toBeUndefined()
  })

  it('does not let an old restored run overwrite a newer invocation for the same item and action', async () => {
    let resolveOldRun!: (run: { commandId: number; status: string; stderr: string }) => void
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ 'o/r#1': { review: 41 } }))
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.ActionRun.mockImplementation(() => new Promise((resolve) => { resolveOldRun = resolve as typeof resolveOldRun }))
    mocks.InvokeAction.mockResolvedValue({ commandId: 42, status: 'done' })
    const get = mountState(); await flushPromises()
    const loading = get().selectSidebar({ type: 'all' }); await flushPromises()
    await get().invokeAction('review')
    resolveOldRun({ commandId: 41, status: 'failed', stderr: 'stale' })
    await loading; await flushPromises()
    expect(get().actionRuns.value.review?.commandId).toBe(42)
    expect(JSON.parse(localStorage.getItem('hive.action-run-ids') ?? '{}')).toEqual({ 'o/r#1': { review: 42 } })
  })

  it('does not let an old not-found response delete a newer invocation for the same item and action', async () => {
    let rejectOldRun!: (error: Error) => void
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ 'o/r#1': { review: 41 } }))
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.ActionRun.mockImplementation(() => new Promise((_, reject) => { rejectOldRun = reject as typeof rejectOldRun }))
    mocks.InvokeAction.mockResolvedValue({ commandId: 42, status: 'done' })
    const get = mountState(); await flushPromises()
    const loading = get().selectSidebar({ type: 'all' }); await flushPromises()
    await get().invokeAction('review')
    rejectOldRun(new Error('command not found'))
    await loading; await flushPromises()
    expect(get().actionRuns.value.review?.commandId).toBe(42)
    expect(JSON.parse(localStorage.getItem('hive.action-run-ids') ?? '{}')).toEqual({ 'o/r#1': { review: 42 } })
  })

  it('does not let an out-of-order restored run appear on the newly selected item', async () => {
    let resolveRun!: (run: { commandId: number; status: string; stderr: string }) => void
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ 'o/r#1': { review: 55 } }))
    mocks.FeedItems.mockResolvedValue([
      { feedId: 'triage/my-prs', itemId: 'o/r#1', unread: false, payload: { id: 'o/r#1', title: 'One', kind: 'PR' } },
      { feedId: 'triage/my-prs', itemId: 'o/r#2', unread: false, payload: { id: 'o/r#2', title: 'Two', kind: 'PR' } },
    ])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell' }])
    mocks.ActionRun.mockImplementation(() => new Promise((resolve) => { resolveRun = resolve as typeof resolveRun }))
    const get = mountState(); await flushPromises()
    const loading = get().selectSidebar({ type: 'all' }); await flushPromises()
    await get().selectItem('o/r#2'); await flushPromises()
    resolveRun({ commandId: 55, status: 'failed', stderr: 'old item only' })
    await loading; await flushPromises()
    expect(get().selectedId.value).toBe('o/r#2')
    expect(get().actionRuns.value.review).toBeUndefined()
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
