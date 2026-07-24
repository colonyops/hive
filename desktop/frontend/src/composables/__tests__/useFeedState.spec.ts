import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { useFeedState } from '../useFeedState'
import type { InboxItem } from '../../types/feed'

const mocks = vi.hoisted(() => ({
  ListFlows: vi.fn(), GetFlow: vi.fn(), CreateFlow: vi.fn(), RenameFlow: vi.fn(), SetFlowEnabled: vi.fn(), DeleteFlow: vi.fn(), GetSidebar: vi.fn(), SaveSidebar: vi.fn(),
  ListInboxItemsByFeed: vi.fn(), ListArchivedInboxItemsByFeed: vi.fn(), ListInboxItemsTrash: vi.fn(), FeedCounts: vi.fn(), MarkInboxItemUnread: vi.fn(), ToggleInboxItemArchived: vi.fn(), ToggleInboxItemIgnored: vi.fn(), InboxItemEvents: vi.fn(),
  ActionViews: vi.fn(), ActionRun: vi.fn(), InvokeAction: vi.fn(), SessionLaunchOptions: vi.fn(), On: vi.fn(), Hide: vi.fn(), OpenURL: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/desktop/flowsservice', () => ({ ListFlows: mocks.ListFlows, GetFlow: mocks.GetFlow, CreateFlow: mocks.CreateFlow, RenameFlow: mocks.RenameFlow, SetFlowEnabled: mocks.SetFlowEnabled, DeleteFlow: mocks.DeleteFlow, GetSidebar: mocks.GetSidebar, SaveSidebar: mocks.SaveSidebar }))
vi.mock('../../../bindings/github.com/colonyops/hive/desktop/pipelineservice', () => ({
  ListInboxItemsByFeed: mocks.ListInboxItemsByFeed, ListArchivedInboxItemsByFeed: mocks.ListArchivedInboxItemsByFeed, ListInboxItemsTrash: mocks.ListInboxItemsTrash, FeedCounts: mocks.FeedCounts,
  MarkInboxItemUnread: mocks.MarkInboxItemUnread, ToggleInboxItemArchived: mocks.ToggleInboxItemArchived, ToggleInboxItemIgnored: mocks.ToggleInboxItemIgnored, InboxItemEvents: mocks.InboxItemEvents,
  ActionViews: mocks.ActionViews, ActionRun: mocks.ActionRun, InvokeAction: mocks.InvokeAction, SessionLaunchOptions: mocks.SessionLaunchOptions,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On }, Window: { Hide: mocks.Hide }, Browser: { OpenURL: mocks.OpenURL }, Call: { ByID: vi.fn() } }))

const flow = { id: 'triage', name: 'Frontend Triage', enabled: true, nodes: [{ id: 'source', type: 'github-source' }, { id: 'my-prs', type: 'feed', name: 'My PRs' }], wires: [] }
function item(id: number, overrides: Partial<InboxItem> = {}): InboxItem {
  return { id, profileId: 'triage', sourceKind: 'github', sourceScope: 'acme/app', externalId: `pr-${id}`, title: `Item ${id}`, url: `https://example.test/${id}`, payload: { id: `pr-${id}`, kind: 'PR', repo: 'acme/app', num: id, author: 'hay', body: `body ${id}`, branch: 'main' }, revision: 1, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: id, ...overrides }
}
function mountState() { let state!: ReturnType<typeof useFeedState>; mount({ setup() { state = useFeedState(); return () => null } }); return () => state }

beforeEach(() => {
  vi.clearAllMocks(); localStorage.clear()
  mocks.ListFlows.mockResolvedValue([{ id: 'triage', name: 'Frontend Triage', enabled: true, valid: true }])
  mocks.GetFlow.mockResolvedValue(flow); mocks.GetSidebar.mockResolvedValue({ items: [] }); mocks.SaveSidebar.mockResolvedValue(undefined)
  mocks.FeedCounts.mockResolvedValue([{ feedId: 'triage/my-prs', total: 3, unread: 2, archived: 1 }])
  mocks.ListInboxItemsByFeed.mockResolvedValue([]); mocks.ListArchivedInboxItemsByFeed.mockResolvedValue([]); mocks.ListInboxItemsTrash.mockResolvedValue([]); mocks.ActionViews.mockResolvedValue([]); mocks.ActionRun.mockResolvedValue({ commandId: 1, status: 'done' }); mocks.SessionLaunchOptions.mockResolvedValue({ repositories: [{ name: 'hive', repository: 'https://github.com/colonyops/hive.git' }], defaultRepository: 'https://github.com/colonyops/hive.git', agents: ['claude'], defaultAgent: 'claude' })
  mocks.MarkInboxItemUnread.mockImplementation(async (id: number, revision: number, unread: boolean) => item(id, { revision: revision + 1, unread }))
  mocks.ToggleInboxItemArchived.mockImplementation(async (id: number, revision: number) => item(id, { revision: revision + 1, archivedAt: Date.now() }))
  mocks.ToggleInboxItemIgnored.mockImplementation(async (id: number, revision: number) => item(id, { revision: revision + 1, ignoredAt: Date.now() }))
  mocks.SetFlowEnabled.mockImplementation(async (id: string, enabled: boolean) => ({ id, name: 'Frontend Triage', enabled, valid: true }))
  mocks.On.mockReturnValue(() => {})
})
afterEach(() => vi.unstubAllGlobals())

describe('useFeedState', () => {
  it('loads flow profiles with feed counts and opens the first feed by default', async () => {
    const get = mountState(); await flushPromises()
    expect(get().activeProfileId.value).toBe('triage')
    expect(get().activeProfile.value?.feeds).toEqual([{ id: 'triage/my-prs', name: 'My PRs', count: 3, newCount: 2, archivedCount: 1, icon: undefined, description: undefined }])
    expect(get().activeProfile.value?.tree).toMatchObject([{ kind: 'feed', feed: { id: 'triage/my-prs' } }])
    // Workspace rollups derive from feed counts; there is no aggregate inbox query.
    expect(get().activeProfile.value).toMatchObject({ totalCount: 3, unreadCount: 2 })
    expect(get().selection.value).toEqual({ type: 'feed', feedId: 'triage/my-prs' })
    expect(mocks.ListInboxItemsByFeed).toHaveBeenCalledWith('triage', 'triage/my-prs', 500)
  })

  it('routes trash and feed selections to their dedicated queries', async () => {
    const get = mountState(); await flushPromises()
    await get().selectSidebar({ type: 'trash' })
    expect(mocks.ListInboxItemsTrash).toHaveBeenCalledWith('triage', 500)
    expect(get().title.value).toBe('Trash')
    await get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' })
    expect(mocks.ListInboxItemsByFeed).toHaveBeenLastCalledWith('triage', 'triage/my-prs', 500)
  })

  it('remembers the last selection per profile and restores it on re-entry', async () => {
    const get = mountState(); await flushPromises()
    await get().selectSidebar({ type: 'trash' })
    expect(JSON.parse(localStorage.getItem('hive.sidebar.last-selection') ?? '{}')).toEqual({ triage: { type: 'trash' } })
    await get().selectProfile('triage')
    expect(get().selection.value).toEqual({ type: 'trash' })
  })

  it('lazy-loads the archived section only when expanded and clears it on selection change', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(1)])
    mocks.ListArchivedInboxItemsByFeed.mockResolvedValue([item(9, { archivedAt: 9, archivedReason: 'merged', unread: false })])
    const get = mountState(); await flushPromises()
    expect(mocks.ListArchivedInboxItemsByFeed).not.toHaveBeenCalled()
    expect(get().archivedCount.value).toBe(1) // divider count comes from feed counts
    await get().toggleArchivedSection()
    expect(mocks.ListArchivedInboxItemsByFeed).toHaveBeenCalledWith('triage', 'triage/my-prs', 500)
    expect(get().visibleArchivedItems.value.map((row) => row.id)).toEqual([9])
    await get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' })
    expect(get().archivedExpanded.value).toBe(false)
    expect(get().visibleArchivedItems.value).toEqual([])
  })

  it('filters trash to ignored items only when the ignored filter is set', async () => {
    mocks.ListInboxItemsTrash.mockResolvedValue([item(1), item(2, { ignoredAt: 7, unread: false })])
    const get = mountState(); await flushPromises()
    await get().selectSidebar({ type: 'trash' })
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([2, 1])
    get().setTrashFilter('ignored')
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([2])
  })

  it('preserves a newer feed reload when an older request resolves late', async () => {
    const handlers: Record<string, () => void> = {}; mocks.On.mockImplementation((name: string, callback: () => void) => { handlers[name] = callback; return () => {} })
    const get = mountState(); await flushPromises()
    const resolve: Array<(value: typeof flow) => void> = []; mocks.GetFlow.mockImplementation(() => new Promise(r => resolve.push(r)))
    handlers['flows:updated']!(); await flushPromises(); handlers['flows:updated']!(); await flushPromises()
    resolve[1]!({ ...flow, nodes: [flow.nodes[0], { ...flow.nodes[1], name: 'New name' }] }); await flushPromises(); resolve[0]!(flow); await flushPromises()
    expect(get().activeProfile.value?.feeds[0]?.name).toBe('New name')
  })

  it('filters and navigates loaded feed rows while retaining SQL order', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(3, { title: 'Bravo', unread: false }), item(2, { title: 'Alpha' }), item(1, { title: 'Bravo follow-up' })])
    const get = mountState(); await flushPromises()
    get().search.value = 'bravo'
    expect(get().visibleItems.value.map(row => row.id)).toEqual([3, 1])
    await get().selectItem(3); await get().selectNext(); expect(get().selectedId.value).toBe(1)
    expect(get().unreadCount.value).toBe(1) // selecting rows marks them read
  })

  it('sorts feed items by newest, oldest, or unread-first recency and persists the choice', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([
      item(1, { title: 'Oldest', unread: false, lastEventAt: 100 }),
      item(3, { title: 'Newest', unread: false, lastEventAt: 300 }),
      item(2, { title: 'Unread', unread: true, lastEventAt: 200 }),
    ])
    const get = mountState(); await flushPromises()
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([3, 2, 1])
    get().setFeedSort('oldest')
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([1, 2, 3])
    get().setFeedSort('unread')
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([2, 3, 1])
    await flushPromises()
    expect(localStorage.getItem('hive.feed.sort')).toBe('unread')
  })

  it('reloads the current selection after an archive toggle moves an item between sections', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(1)])
    const get = mountState(); await flushPromises()
    const before = mocks.ListInboxItemsByFeed.mock.calls.length
    await get().toggleArchive(get().items.value[0]!)
    expect(mocks.ListInboxItemsByFeed.mock.calls.length).toBeGreaterThan(before)

    mocks.ListInboxItemsTrash.mockResolvedValue([item(2, { ignoredAt: 5 })])
    await get().selectSidebar({ type: 'trash' })
    const trashBefore = mocks.ListInboxItemsTrash.mock.calls.length
    await get().toggleArchive(get().items.value[0]!)
    expect(mocks.ListInboxItemsTrash.mock.calls.length).toBeGreaterThan(trashBefore)
  })

  it('marks reads with item revision and reloads after stale failures', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(5)])
    const get = mountState(); await flushPromises(); await get().selectItem(5)
    expect(mocks.MarkInboxItemUnread).toHaveBeenCalledWith(5, 1, false)
    expect(get().items.value[0]?.revision).toBe(2)
    mocks.MarkInboxItemUnread.mockRejectedValueOnce(new Error('stale'))
    await get().markItemUnread(get().items.value[0]!, true)
    expect(mocks.ListInboxItemsByFeed).toHaveBeenLastCalledWith('triage', 'triage/my-prs', 500)
  })

  it('loads action runs by selected item id and does not let old action responses replace a new selection', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(1), item(2)])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false }])
    let resolve!: (value: { commandId: number; status: string }) => void
    mocks.InvokeAction.mockReturnValue(new Promise(r => { resolve = r }))
    const get = mountState(); await flushPromises(); await get().selectItem(1)
    const invocation = get().invokeAction('review'); await get().selectItem(2); resolve({ commandId: 9, status: 'done' }); await invocation
    expect(get().actionRuns.value).toEqual({})
  })

  it('opens the selected item URL in the browser', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(3)])
    const get = mountState(); await flushPromises(); await get().openSelectedInBrowser()
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://example.test/3')
  })

  it('reconciles saved sidebar folders and persists reordered node ids', async () => {
    mocks.GetSidebar.mockResolvedValue({ items: [{ folder: { id: 'work', name: 'Work', feeds: ['my-prs'] } }] })
    const get = mountState(); await flushPromises()
    expect(get().activeProfile.value?.tree).toMatchObject([{ kind: 'folder', folder: { id: 'work', feeds: [{ id: 'triage/my-prs' }] } }])
    await get().reorderFeeds('triage', [{ kind: 'folder', folder: { id: 'work', name: 'Work', feeds: [get().activeProfile.value!.feeds[0]!] } }])
    expect(mocks.SaveSidebar).toHaveBeenCalledWith('triage', { items: [{ folder: { id: 'work', name: 'Work', feeds: ['my-prs'] } }] })
  })

  it('toggles profile enablement while keeping its selection', async () => {
    const get = mountState(); await flushPromises()
    expect(await get().setProfileEnabled('triage', false)).toBe(true)
    expect(mocks.SetFlowEnabled).toHaveBeenCalledWith('triage', false)
    expect(get().activeProfileId.value).toBe('triage')
    expect(get().activeProfile.value?.enabled).toBe(false)

    mocks.SetFlowEnabled.mockRejectedValueOnce(new Error('disk is read-only'))
    expect(await get().setProfileEnabled('triage', true)).toBe(false)
    expect(get().toggleProfileError.value).toBe('disk is read-only')
    expect(get().activeProfile.value?.enabled).toBe(false)
  })

  it('creates, renames, rejects failed renames, and deletes flow-backed profiles', async () => {
    mocks.CreateFlow.mockResolvedValue({ id: 'new', name: 'New', enabled: true, valid: true })
    mocks.RenameFlow.mockResolvedValue({ id: 'triage', name: 'Team Triage', enabled: true, valid: true })
    mocks.DeleteFlow.mockResolvedValue(undefined)
    const get = mountState(); await flushPromises()
    await get().createProfile('New')
    expect(mocks.CreateFlow).toHaveBeenCalledWith('New')
    expect(get().profiles.value.some((profile) => profile.id === 'new')).toBe(true)
    await get().selectProfile('triage')
    expect(await get().renameProfile('triage', ' Team Triage ')).toBe(true)
    expect(get().activeProfile.value).toMatchObject({ name: 'Team Triage', letter: 'T' })
    mocks.RenameFlow.mockRejectedValueOnce(new Error('disk is read-only'))
    expect(await get().renameProfile('triage', 'Nope')).toBe(false)
    expect(get().renameProfileError.value).toBe('disk is read-only')
    await get().deleteProfile('triage')
    expect(mocks.DeleteFlow).toHaveBeenCalledWith('triage')
  })

  it('opens arbitrary URLs and reports a selected item without a URL', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(1, { url: '' })])
    const get = mountState(); await flushPromises()
    await get().openSelectedInBrowser()
    expect(mocks.OpenURL).not.toHaveBeenCalled()
    expect(get().toasts.value.some((toast) => toast.message === 'No link available for this item')).toBe(true)
    await get().openUrl('https://example.test/docs')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://example.test/docs')
  })

  it('invokes actions by authoritative numeric item id and preserves success/failure feedback', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false }])
    mocks.InvokeAction.mockResolvedValueOnce({ commandId: 17, status: 'done' })
    const get = mountState(); await flushPromises()
    await get().invokeAction('review')
    expect(mocks.InvokeAction).toHaveBeenCalledWith('review', 7, {})
    expect(get().actionRuns.value.review?.commandId).toBe(17)
    expect(get().toasts.value.some((toast) => toast.message === 'Review completed')).toBe(true)
    mocks.InvokeAction.mockResolvedValueOnce({ commandId: 18, status: 'failed', error: 'command exited 1', stderr: 'bad input' })
    await get().invokeAction('review')
    expect(get().actionRuns.value.review).toMatchObject({ commandId: 18, status: 'failed', stderr: 'bad input' })
    expect(get().toasts.value.some((toast) => toast.severity === 'error' && toast.message === 'command exited 1')).toBe(true)
  })

  it('names published messages and opens interactive session launch actions before invocation', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([{ id: 'notify', label: 'Notify', type: 'publish-message', showInDetail: true, requiresSessionInput: false }])
    mocks.InvokeAction.mockResolvedValueOnce({ commandId: 18, status: 'done', result: { message: { topic: 'agent.session.inbox', sender: 'hive-desktop' } } })
    const get = mountState(); await flushPromises()
    await get().invokeAction('notify')
    expect(get().toasts.value.some((toast) => toast.message === 'Published message to agent.session.inbox as hive-desktop')).toBe(true)
    mocks.ActionViews.mockResolvedValue([{ id: 'launch', label: 'Launch', type: 'launch-session', showInDetail: true, requiresSessionInput: true }])
    await get().selectItem(7)
    await get().invokeAction('launch')
    expect(mocks.SessionLaunchOptions).toHaveBeenCalledOnce()
    expect(get().sessionLaunchAction.value?.id).toBe('launch')
    mocks.InvokeAction.mockResolvedValueOnce({ commandId: 19, status: 'done', result: { session: { id: 'session-1', name: 'review-pr-7' } } })
    await get().submitSessionLaunch({ name: 'review-pr-7', repository: 'https://github.com/colonyops/hive.git', agent: 'claude' })
    expect(mocks.InvokeAction).toHaveBeenLastCalledWith('launch', 7, { session: { name: 'review-pr-7', repository: 'https://github.com/colonyops/hive.git', agent: 'claude' } })
    expect(get().toasts.value.some((toast) => toast.message === 'Created session review-pr-7 (session-1)')).toBe(true)
  })

  it('scopes persisted action runs to the numeric item that owns them', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ '1': { review: 41 } }))
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(1), item(2)])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false }])
    mocks.ActionRun.mockResolvedValue({ commandId: 41, status: 'failed', stderr: 'details' })
    const get = mountState(); await flushPromises()
    expect(mocks.ActionRun).toHaveBeenCalledWith(41)
    expect(get().actionRuns.value.review?.stderr).toBe('details')
    await get().selectItem(2); await flushPromises()
    expect(get().actionRuns.value.review).toBeUndefined()
    await get().selectItem(1); await flushPromises()
    expect(mocks.ActionRun).toHaveBeenCalledTimes(2)
  })

  it('rejects malformed persisted action run ids without restoring them', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ '1': { review: '41', zero: 0 }, bad: [] }))
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(1)])
    mocks.ActionViews.mockResolvedValue([{ id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false }])
    const get = mountState(); await flushPromises()
    expect(get().actionRuns.value).toEqual({})
    expect(mocks.ActionRun).not.toHaveBeenCalled()
  })

  it('keeps the newer list when an older selection request resolves late', async () => {
    const get = mountState(); await flushPromises()
    const resolveFeed: Array<(rows: InboxItem[]) => void> = []
    const resolveTrash: Array<(rows: InboxItem[]) => void> = []
    mocks.ListInboxItemsByFeed.mockImplementation(() => new Promise<InboxItem[]>(done => resolveFeed.push(done)))
    mocks.ListInboxItemsTrash.mockImplementation(() => new Promise<InboxItem[]>(done => resolveTrash.push(done)))
    const feed = get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' }); await flushPromises()
    const trash = get().selectSidebar({ type: 'trash' }); await flushPromises()
    resolveTrash[0]!([item(2, { ignoredAt: 2 })]); await trash
    resolveFeed[0]!([item(1)]); await feed
    expect(get().selection.value).toEqual({ type: 'trash' })
    expect(get().items.value.map((row) => row.id)).toEqual([2])
  })

  it('renders observed events oldest-to-newest even though the storage read is newest-first', async () => {
    mocks.InboxItemEvents.mockResolvedValue([
      { id: 3, itemId: 1, kind: 'closed', transition: 'closed', attention: 'none', summary: 'newest', createdAt: 3 },
      { id: 2, itemId: 1, kind: 'updated', transition: 'updated', attention: 'activity', summary: 'middle', createdAt: 2 },
      { id: 1, itemId: 1, kind: 'created', transition: 'created', attention: 'activity', summary: 'oldest', createdAt: 1 },
    ])
    const get = mountState(); await flushPromises()
    expect((await get().loadEvents(1)).map((event) => event.summary)).toEqual(['oldest', 'middle', 'newest'])
  })

  it('retains load errors, clears stale rows, and retries the current selection on refresh', async () => {
    mocks.ListInboxItemsByFeed.mockRejectedValueOnce(new Error('offline'))
    const get = mountState(); await flushPromises()
    expect(get().loadError.value).toBe("Can't load inbox items right now.")
    expect(get().items.value).toEqual([])
    mocks.ListInboxItemsTrash.mockResolvedValue([item(9, { ignoredAt: 9 })])
    await get().selectSidebar({ type: 'trash' })
    expect(get().items.value[0]).toMatchObject({ id: 9 })
    await get().refresh()
    expect(mocks.ListInboxItemsTrash).toHaveBeenLastCalledWith('triage', 500)
  })

  it('navigates only across the searched unread subset as selected rows become read', async () => {
    mocks.ListInboxItemsByFeed.mockResolvedValue([item(3, { title: 'Onboard', unread: true }), item(2, { title: 'Deploy', unread: true }), item(1, { title: 'Onload', unread: true })])
    const get = mountState(); await flushPromises()
    await get().selectUnreadView()
    get().search.value = 'on'
    await get().selectItem(3)
    await get().selectNext()
    expect(get().selectedId.value).toBe(1)
    // Selecting the landing row marks it read, so it drops out of the
    // unread-only subset without moving the cursor to an unrelated row.
    expect(get().visibleItems.value).toEqual([])
    await get().selectPrev()
    expect(get().selectedId.value).toBe(1)
  })
})
