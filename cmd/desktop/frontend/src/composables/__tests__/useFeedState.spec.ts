import { readFileSync } from 'node:fs'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { useFeedState } from '../useFeedState'
import { resetToastsForTests } from '../useToasts'
import type { InboxItem } from '../../types/feed'

const mocks = vi.hoisted(() => ({
  ListFlows: vi.fn(),
  GetFlow: vi.fn(),
  CreateFlow: vi.fn(),
  SeedStarterFlow: vi.fn(),
  RenameFlow: vi.fn(),
  SetFlowEnabled: vi.fn(),
  DeleteFlow: vi.fn(),
  GetSidebar: vi.fn(),
  SaveSidebar: vi.fn(),
  ListByFeed: vi.fn(),
  ListArchivedByFeed: vi.fn(),
  ListTrash: vi.fn(),
  FeedCounts: vi.fn(),
  SetUnread: vi.fn(),
  MarkRead: vi.fn(),
  ToggleArchived: vi.fn(),
  ToggleIgnored: vi.fn(),
  Events: vi.fn(),
  ActionViews: vi.fn(),
  ActionRun: vi.fn(),
  InvokeAction: vi.fn(),
  RenderClipboardAction: vi.fn(),
  SessionLaunchOptions: vi.fn(),
  On: vi.fn(),
  Hide: vi.fn(),
  OpenURL: vi.fn(),
  SetText: vi.fn(),
  notify: vi.fn(),
  RefreshSources: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/flowsservice', () => ({
  ListFlows: mocks.ListFlows,
  GetFlow: mocks.GetFlow,
  CreateFlow: mocks.CreateFlow,
  SeedStarterFlow: mocks.SeedStarterFlow,
  RenameFlow: mocks.RenameFlow,
  SetFlowEnabled: mocks.SetFlowEnabled,
  DeleteFlow: mocks.DeleteFlow,
  GetSidebar: mocks.GetSidebar,
  SaveSidebar: mocks.SaveSidebar,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice', () => ({
  ListByFeed: mocks.ListByFeed,
  ListArchivedByFeed: mocks.ListArchivedByFeed,
  ListTrash: mocks.ListTrash,
  FeedCounts: mocks.FeedCounts,
  SetUnread: mocks.SetUnread,
  MarkRead: mocks.MarkRead,
  ToggleArchived: mocks.ToggleArchived,
  ToggleIgnored: mocks.ToggleIgnored,
  Events: mocks.Events,
  ActionViews: mocks.ActionViews,
  ActionRun: mocks.ActionRun,
  InvokeAction: mocks.InvokeAction,
  RenderClipboardAction: mocks.RenderClipboardAction,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionLaunchOptions: mocks.SessionLaunchOptions,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sourcesservice', () => ({
  Refresh: mocks.RefreshSources,
}))
vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Window: { Hide: mocks.Hide },
  Browser: { OpenURL: mocks.OpenURL },
  Clipboard: { SetText: mocks.SetText },
  Call: { ByID: vi.fn() },
}))
vi.mock('../useNotify', () => ({ useNotify: () => ({ notify: mocks.notify }) }))

const flow = {
  id: 'triage',
  name: 'Frontend Triage',
  enabled: true,
  nodes: [
    { id: 'source', type: 'sources.github' },
    { id: 'my-prs', type: 'feed', name: 'My PRs' },
  ],
  wires: [],
}
function item(id: number, overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    id,
    profileId: 'triage',
    sourceKind: 'github',
    sourceScope: 'acme/app',
    externalId: `pr-${id}`,
    title: `Item ${id}`,
    url: `https://example.test/${id}`,
    payload: {
      id: `pr-${id}`,
      kind: 'PR',
      repo: 'acme/app',
      num: id,
      author: 'hay',
      body: `body ${id}`,
      branch: 'main',
    },
    revision: 1,
    unread: true,
    lifecycle: 'active',
    firstSeenAt: 1,
    lastEventAt: id,
    ...overrides,
  }
}
function mountState() {
  let state!: ReturnType<typeof useFeedState>
  mount({
    setup() {
      state = useFeedState()
      return () => null
    },
  })
  return () => state
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  resetToastsForTests()
  mocks.notify.mockResolvedValue(undefined)
  mocks.SetText.mockResolvedValue(undefined)
  mocks.ListFlows.mockResolvedValue([{ id: 'triage', name: 'Frontend Triage', enabled: true, valid: true }])
  mocks.GetFlow.mockResolvedValue(flow)
  mocks.GetSidebar.mockResolvedValue({ items: [] })
  mocks.SaveSidebar.mockResolvedValue(undefined)
  mocks.FeedCounts.mockResolvedValue([{ feedId: 'triage/my-prs', total: 3, unread: 2, archived: 1 }])
  mocks.ListByFeed.mockResolvedValue([])
  mocks.ListArchivedByFeed.mockResolvedValue([])
  mocks.ListTrash.mockResolvedValue([])
  mocks.ActionViews.mockResolvedValue([])
  mocks.ActionRun.mockResolvedValue({ commandId: 1, status: 'done' })
  mocks.SessionLaunchOptions.mockResolvedValue({
    repositories: [{ name: 'hive', repository: 'https://github.com/hay-kot/hive-desktop.git' }],
    defaultRepository: 'https://github.com/hay-kot/hive-desktop.git',
    agents: ['claude'],
    defaultAgent: 'claude',
  })
  mocks.MarkRead.mockResolvedValue(0)
  mocks.SetUnread.mockImplementation(async (id: number, revision: number, unread: boolean) =>
    item(id, { revision: revision + 1, unread }),
  )
  mocks.ToggleArchived.mockImplementation(async (id: number, revision: number) =>
    item(id, { revision: revision + 1, archivedAt: Date.now() }),
  )
  mocks.ToggleIgnored.mockImplementation(async (id: number, revision: number) =>
    item(id, { revision: revision + 1, ignoredAt: Date.now() }),
  )
  mocks.SetFlowEnabled.mockImplementation(async (id: string, enabled: boolean) => ({
    id,
    name: 'Frontend Triage',
    enabled,
    valid: true,
  }))
  mocks.On.mockReturnValue(() => {})
  mocks.RefreshSources.mockResolvedValue({ sources: 1, appended: 0, failed: 0 })
})
afterEach(() => vi.unstubAllGlobals())

describe('useFeedState', () => {
  it('keeps durable outcomes on notify and reserves showToast for ephemeral feedback', () => {
    const source = readFileSync('src/composables/useFeedState.ts', 'utf8')
    expect(source).not.toContain('recordActivity')
    expect(source.match(/showToast\(/g)).toHaveLength(10)
    for (const title of [
      'Sidebar layout save failed',
      'Profile renamed',
      'Profile enabled',
      'Profile disabled',
      "Couldn't delete profile",
      'Profile deleted',
    ]) {
      expect(source).toContain(title)
    }
  })

  it('loads flow profiles with feed counts and opens the first feed by default', async () => {
    const get = mountState()
    await flushPromises()
    expect(get().activeProfileId.value).toBe('triage')
    expect(get().activeProfile.value?.feeds).toEqual([
      {
        id: 'triage/my-prs',
        name: 'My PRs',
        count: 3,
        newCount: 2,
        archivedCount: 1,
        icon: undefined,
        description: undefined,
      },
    ])
    expect(get().activeProfile.value?.tree).toMatchObject([{ kind: 'feed', feed: { id: 'triage/my-prs' } }])
    // Profile rollups derive from feed counts; there is no aggregate inbox query.
    expect(get().activeProfile.value).toMatchObject({ totalCount: 3, unreadCount: 2 })
    expect(get().selection.value).toEqual({ type: 'feed', feedId: 'triage/my-prs' })
    expect(mocks.ListByFeed).toHaveBeenCalledWith('triage', 'triage/my-prs', 500)
  })

  it('routes trash and feed selections to their dedicated queries', async () => {
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'trash' })
    expect(mocks.ListTrash).toHaveBeenCalledWith('triage', 500)
    expect(get().title.value).toBe('Trash')
    await get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' })
    expect(mocks.ListByFeed).toHaveBeenLastCalledWith('triage', 'triage/my-prs', 500)
  })

  it('remembers the last selection per profile and restores it on re-entry', async () => {
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'trash' })
    expect(JSON.parse(localStorage.getItem('hive.sidebar.last-selection') ?? '{}')).toEqual({
      triage: { type: 'trash' },
    })
    await get().selectProfile('triage')
    expect(get().selection.value).toEqual({ type: 'trash' })
  })

  it('lazy-loads the archived section only when expanded and clears it on selection change', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1)])
    mocks.ListArchivedByFeed.mockResolvedValue([item(9, { archivedAt: 9, archivedReason: 'merged', unread: false })])
    const get = mountState()
    await flushPromises()
    expect(mocks.ListArchivedByFeed).not.toHaveBeenCalled()
    expect(get().archivedCount.value).toBe(1) // divider count comes from feed counts
    await get().toggleArchivedSection()
    expect(mocks.ListArchivedByFeed).toHaveBeenCalledWith('triage', 'triage/my-prs', 500)
    expect(get().visibleArchivedItems.value.map((row) => row.id)).toEqual([9])
    await get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' })
    expect(get().archivedExpanded.value).toBe(false)
    expect(get().visibleArchivedItems.value).toEqual([])
  })

  it('filters trash to ignored items only when the ignored filter is set', async () => {
    mocks.ListTrash.mockResolvedValue([item(1), item(2, { ignoredAt: 7, unread: false })])
    const get = mountState()
    await flushPromises()
    await get().selectSidebar({ type: 'trash' })
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([2, 1])
    get().setTrashFilter('ignored')
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([2])
  })

  it('preserves a newer feed reload when an older request resolves late', async () => {
    const handlers: Record<string, () => void> = {}
    mocks.On.mockImplementation((name: string, callback: () => void) => {
      handlers[name] = callback
      return () => {}
    })
    const get = mountState()
    await flushPromises()
    const resolve: Array<(value: typeof flow) => void> = []
    mocks.GetFlow.mockImplementation(() => new Promise((r) => resolve.push(r)))
    handlers['flows:updated']!()
    await flushPromises()
    handlers['flows:updated']!()
    await flushPromises()
    resolve[1]!({ ...flow, nodes: [flow.nodes[0], { ...flow.nodes[1], name: 'New name' }] })
    await flushPromises()
    resolve[0]!(flow)
    await flushPromises()
    expect(get().activeProfile.value?.feeds[0]?.name).toBe('New name')
  })

  // The app writes the default profile at startup, and the flows watcher
  // reports that write while the first load is still reading. App.vue holds
  // the shell on "Loading…" until profilesLoaded, so a reload that supersedes
  // the first load must finish it.
  it('finishes the first load when flows:updated lands while it is reading', async () => {
    const handlers: Record<string, () => void> = {}
    mocks.On.mockImplementation((name: string, callback: () => void) => {
      handlers[name] = callback
      return () => {}
    })
    let releaseFirst!: (value: unknown) => void
    mocks.ListFlows.mockImplementationOnce(
      () =>
        new Promise((r) => {
          releaseFirst = r
        }),
    )
    const get = mountState()
    await flushPromises()

    handlers['flows:updated']!()
    await flushPromises()
    releaseFirst([{ id: 'triage', name: 'Frontend Triage', enabled: true, valid: true }])
    await flushPromises()

    expect(get().profilesLoaded.value).toBe(true)
    expect(get().activeProfileId.value).toBe('triage')
    expect(get().selection.value).toEqual({ type: 'feed', feedId: 'triage/my-prs' })
  })

  // A profile whose flow has no feed nodes has to be distinguishable from
  // one whose feeds simply have not been read yet — App.vue's empty state
  // hangs off that difference, and a stub mid-reload must not trip it.
  it('leaves a reloading profile without a tree until its feeds are read', async () => {
    const handlers: Record<string, () => void> = {}
    mocks.On.mockImplementation((name: string, callback: () => void) => {
      handlers[name] = callback
      return () => {}
    })
    const get = mountState()
    await flushPromises()
    expect(get().activeProfile.value?.tree).toBeDefined()

    let release!: (value: typeof flow) => void
    mocks.GetFlow.mockImplementation(
      () =>
        new Promise((r) => {
          release = r
        }),
    )
    handlers['flows:updated']!()
    await flushPromises()
    expect(get().activeProfile.value?.tree).toBeUndefined()

    release({ ...flow, nodes: [flow.nodes[0]] })
    await flushPromises()
    expect(get().activeProfile.value?.tree).toEqual([])
    expect(get().activeProfile.value?.feeds).toHaveLength(0)
  })

  it('seeds the starter flow into a profile and re-reads its feeds', async () => {
    mocks.SeedStarterFlow.mockResolvedValue({ id: 'triage', name: 'Frontend Triage', enabled: true, valid: true })
    const get = mountState()
    await flushPromises()
    const flowReads = mocks.GetFlow.mock.calls.length

    await get().seedStarterFlow('triage')
    expect(mocks.SeedStarterFlow).toHaveBeenCalledWith('triage')
    // The sidebar's feeds must be readable when this resolves, not whenever
    // the backend's flows:updated happens to land.
    expect(mocks.GetFlow.mock.calls.length).toBeGreaterThan(flowReads)
  })

  it('filters and navigates loaded feed rows while retaining SQL order', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(3, { title: 'Bravo', unread: false }),
      item(2, { title: 'Alpha' }),
      item(1, { title: 'Bravo follow-up' }),
    ])
    const get = mountState()
    await flushPromises()
    get().search.value = 'bravo'
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([3, 1])
    await get().selectItem(3)
    await get().selectNext()
    expect(get().selectedId.value).toBe(1)
    expect(get().unreadCount.value).toBe(1) // selecting rows marks them read
  })

  it('keeps hidden selections and orders selected items by the rendered feed order', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(1, { title: 'Old read', unread: false, lastEventAt: 100 }),
      item(3, { title: 'Newest match', lastEventAt: 300 }),
      item(2, { title: 'Middle match', lastEventAt: 200 }),
    ])
    const get = mountState()
    await flushPromises()
    get().enterItemSelection()
    get().toggleItemSelection(1)
    get().toggleItemSelection(3)
    get().toggleItemSelection(2)
    expect(get().selectedItems.value.map((row) => row.id)).toEqual([3, 2, 1])

    get().search.value = 'match'
    get().unreadOnly.value = true
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([3, 2])
    expect(get().selectedItemIDs.value).toEqual([1, 3, 2])

    get().setFeedSort('oldest')
    expect(get().selectedItems.value.map((row) => row.id)).toEqual([1, 2, 3])
  })

  it('copies selected item contents as one block in rendered feed order', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1), item(2)])
    const get = mountState()
    await flushPromises()
    get().enterItemSelection()
    get().toggleItemSelection(1)
    get().toggleItemSelection(2)

    await get().copySelectedItemContents()

    expect(mocks.SetText).toHaveBeenCalledWith(
      'Item 2\nacme/app #2 · https://example.test/2\n\nbody 2\n\nItem 1\nacme/app #1 · https://example.test/1\n\nbody 1',
    )
    expect(get().toasts.value.at(-1)?.message).toBe('2 items copied')
  })

  it('loads common clipboard actions and renders one entry per selected item', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1), item(2)])
    const copyAction = {
      id: 'copy-checkout',
      label: 'Copy checkout commands',
      type: 'clipboard',
      showInDetail: true,
      requiresSessionInput: false,
    }
    mocks.ActionViews.mockImplementation(async (itemID: number) =>
      itemID === 1 ? [copyAction] : [copyAction, { ...copyAction, id: 'copy-url' }],
    )
    mocks.RenderClipboardAction.mockResolvedValue('checkout 2\n\ncheckout 1')
    const get = mountState()
    await flushPromises()
    get().enterItemSelection()
    get().toggleItemSelection(1)
    get().toggleItemSelection(2)
    await flushPromises()

    expect(mocks.ActionViews).toHaveBeenCalledWith(2)
    expect(mocks.ActionViews).toHaveBeenCalledWith(1)
    expect(get().selectionActions.value.map((action) => action.id)).toEqual(['copy-checkout'])

    await get().invokeSelectionAction('copy-checkout')
    expect(mocks.RenderClipboardAction).toHaveBeenCalledWith('copy-checkout', [2, 1], {})
    expect(mocks.SetText).toHaveBeenCalledWith('checkout 2\n\ncheckout 1')
  })

  it('collects clipboard action inputs once for the whole selection', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1), item(2)])
    mocks.ActionViews.mockResolvedValue([
      {
        id: 'silence',
        label: 'Silence alerts',
        type: 'clipboard',
        showInDetail: true,
        requiresSessionInput: false,
        inputs: [
          {
            name: 'reason',
            label: 'Reason',
            type: 'text',
            required: true,
            default: '',
            placeholder: '',
            options: null,
          },
        ],
      },
    ])
    mocks.RenderClipboardAction.mockResolvedValue('silence 2\n\nsilence 1')
    const get = mountState()
    await flushPromises()
    get().enterItemSelection()
    get().toggleItemSelection(1)
    get().toggleItemSelection(2)
    await flushPromises()

    await get().invokeSelectionAction('silence')
    expect(mocks.RenderClipboardAction).not.toHaveBeenCalled()
    expect(get().actionInputsAction.value?.id).toBe('silence')

    await get().submitActionInputs({ reason: 'flapping' })
    expect(mocks.RenderClipboardAction).toHaveBeenCalledWith('silence', [2, 1], { reason: 'flapping' })
  })

  it('preserves selection on same-destination reloads and prunes items that disappear', async () => {
    mocks.ListByFeed.mockResolvedValue([item(2), item(1)])
    const get = mountState()
    await flushPromises()
    get().enterItemSelection()
    get().toggleItemSelection(1)
    get().toggleItemSelection(2)

    mocks.ListByFeed.mockResolvedValue([item(2)])
    await get().refresh()
    expect(get().selectedItemIDs.value).toEqual([2])
    expect(get().itemSelectionActive.value).toBe(true)

    await get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' })
    expect(get().selectedItemIDs.value).toEqual([2])
    expect(get().itemSelectionActive.value).toBe(true)

    await get().selectSidebar({ type: 'trash' })
    expect(get().selectedItemIDs.value).toEqual([])
    expect(get().itemSelectionActive.value).toBe(false)
  })

  it('keeps a selected item when a reload moves it into archived rows', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1)])
    const get = mountState()
    await flushPromises()
    get().enterItemSelection()
    get().toggleItemSelection(1)

    mocks.ListByFeed.mockResolvedValue([])
    mocks.ListArchivedByFeed.mockResolvedValue([item(1, { archivedAt: 10 })])
    await get().refresh()

    expect(mocks.ListArchivedByFeed).toHaveBeenCalledWith('triage', 'triage/my-prs', 500)
    expect(get().selectedItemIDs.value).toEqual([1])
    expect(get().selectedItems.value.map((row) => row.id)).toEqual([1])
  })

  it('offers distinct authors from loaded items across sources, including archives', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(6, { payload: { author: ' zoe ' } }),
      item(5, { sourceKind: 'exec', payload: { author: 'amy' } }),
      item(4, { payload: { author: 'zoe' } }),
      item(3, { payload: { author: '  ' } }),
      item(2, { payload: { author: { login: 'unknown' } } }),
      item(1, { payload: null }),
    ])
    mocks.ListArchivedByFeed.mockResolvedValue([item(9, { payload: { author: 'ben' }, archivedAt: 9 })])
    const get = mountState()
    await flushPromises()
    expect(get().authors.value).toEqual(['amy', 'zoe'])
    get().search.value = 'no match'
    expect(get().authors.value).toEqual(['amy', 'zoe'])
    await get().toggleArchivedSection()
    expect(get().authors.value).toEqual(['amy', 'ben', 'zoe'])
  })

  it('combines author, search and unread filters and keeps navigation inside the result', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(5, { title: 'Fix first', payload: { author: 'amy' } }),
      item(4, { title: 'Fix other author', payload: { author: 'ben' } }),
      item(3, { title: 'Fix read', payload: { author: 'amy' }, unread: false }),
      item(2, { title: 'Unrelated', payload: { author: 'amy' } }),
      item(1, { title: 'Fix last', payload: { author: 'amy' } }),
    ])
    const get = mountState()
    await flushPromises()
    get().authorFilter.value = 'amy'
    get().search.value = 'fix'
    get().unreadOnly.value = true
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([5, 1])
    await get().selectItem(5)
    await get().selectNext()
    expect(get().selectedId.value).toBe(1)
    expect(get().visibleItems.value).toEqual([])
    await get().selectSidebar({ type: 'trash' })
    expect(get().authorFilter.value).toBe('')
  })

  it('keeps the author filter when switching to unread items', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(3, { payload: { author: 'ben' } }),
      item(2, { payload: { author: 'amy' } }),
      item(1, { unread: false, payload: { author: 'amy' } }),
    ])
    const get = mountState()
    await flushPromises()
    await get().selectItem(1)
    get().authorFilter.value = 'amy'
    await get().selectUnreadView()
    expect(get().selectedId.value).toBe(2)
  })

  it('filters archived authors and can clear a selection after that author disappears', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1, { payload: { author: 'amy' } })])
    mocks.ListArchivedByFeed.mockResolvedValue([
      item(9, { payload: { author: 'amy' }, archivedAt: 9 }),
      item(8, { payload: { author: 'ben' }, archivedAt: 8 }),
    ])
    const get = mountState()
    await flushPromises()
    await get().toggleArchivedSection()
    get().authorFilter.value = 'amy'
    expect(get().visibleArchivedItems.value.map((row) => row.id)).toEqual([9])
    mocks.ListByFeed.mockResolvedValue([item(2, { payload: {} })])
    mocks.ListArchivedByFeed.mockResolvedValue([])
    await get().refresh()
    expect(get().visibleItems.value).toEqual([])
    expect(get().authorFilter.value).toBe('amy')
    get().authorFilter.value = ''
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([2])
  })

  it('sorts feed items by newest, oldest, or unread-first recency and persists the choice', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(1, { title: 'Oldest', unread: false, lastEventAt: 100 }),
      item(3, { title: 'Newest', unread: false, lastEventAt: 300 }),
      item(2, { title: 'Unread', unread: true, lastEventAt: 200 }),
    ])
    const get = mountState()
    await flushPromises()
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([3, 2, 1])
    get().setFeedSort('oldest')
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([1, 2, 3])
    get().setFeedSort('unread')
    expect(get().visibleItems.value.map((row) => row.id)).toEqual([2, 3, 1])
    await flushPromises()
    expect(localStorage.getItem('hive.feed.sort')).toBe('unread')
  })

  it('reloads the current selection after an archive toggle moves an item between sections', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1)])
    const get = mountState()
    await flushPromises()
    const before = mocks.ListByFeed.mock.calls.length
    await get().toggleArchive(get().items.value[0]!)
    expect(mocks.ListByFeed.mock.calls.length).toBeGreaterThan(before)

    mocks.ListTrash.mockResolvedValue([item(2, { ignoredAt: 5 })])
    await get().selectSidebar({ type: 'trash' })
    const trashBefore = mocks.ListTrash.mock.calls.length
    await get().toggleArchive(get().items.value[0]!)
    expect(mocks.ListTrash.mock.calls.length).toBeGreaterThan(trashBefore)
  })

  it('marks reads with item revision and reloads after stale failures', async () => {
    mocks.ListByFeed.mockResolvedValue([item(5)])
    const get = mountState()
    await flushPromises()
    await get().selectItem(5)
    expect(mocks.SetUnread).toHaveBeenCalledWith(5, 1, false)
    expect(get().items.value[0]?.revision).toBe(2)
    mocks.SetUnread.mockRejectedValueOnce(new Error('stale'))
    await get().markItemUnread(get().items.value[0]!, true)
    expect(mocks.ListByFeed).toHaveBeenLastCalledWith('triage', 'triage/my-prs', 500)
  })

  it('clears a whole scope in one write and re-reads the list and the sidebar counts', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1), item(2)])
    mocks.MarkRead.mockResolvedValue(2)
    const get = mountState()
    await flushPromises()
    const listsBefore = mocks.ListByFeed.mock.calls.length
    const countsBefore = mocks.FeedCounts.mock.calls.length

    await get().markAllRead('triage/my-prs')

    expect(mocks.MarkRead).toHaveBeenCalledWith('triage', 'triage/my-prs')
    expect(mocks.SetUnread).not.toHaveBeenCalled() // one bulk write, not one per row
    expect(mocks.ListByFeed.mock.calls.length).toBeGreaterThan(listsBefore)
    expect(mocks.FeedCounts.mock.calls.length).toBeGreaterThan(countsBefore)
    expect(get().toasts.value.map((toast) => toast.message)).toEqual(['Marked 2 items as read'])
  })

  it('marks every feed in the profile when no feed is named, and reports an empty clear', async () => {
    mocks.MarkRead.mockResolvedValue(0)
    const get = mountState()
    await flushPromises()
    await get().markAllRead(null)
    expect(mocks.MarkRead).toHaveBeenCalledWith('triage', '')
    expect(get().toasts.value.map((toast) => toast.message)).toEqual(['Nothing to mark as read'])
  })

  it('surfaces a failed bulk clear and still refreshes rather than leaving a half-stale list', async () => {
    mocks.MarkRead.mockRejectedValue(new Error('locked'))
    const get = mountState()
    await flushPromises()
    const listsBefore = mocks.ListByFeed.mock.calls.length
    await get().markAllRead('triage/my-prs')
    expect(get().toasts.value.map((toast) => toast.message)).toEqual(['Could not mark items as read'])
    expect(mocks.ListByFeed.mock.calls.length).toBeGreaterThan(listsBefore)
  })

  it('reads the scope unread count off the feed counts the sidebar already shows', async () => {
    const get = mountState()
    await flushPromises()
    expect(get().unreadInScope('triage/my-prs')).toBe(2)
    expect(get().unreadInScope(null)).toBe(2)
    expect(get().unreadInScope('triage/gone')).toBe(0)
  })

  it('applies a read write back onto the archived row so its next write is not stale', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1)])
    mocks.ListArchivedByFeed.mockResolvedValue([item(9, { archivedAt: 9, archivedReason: 'manual' })])
    const get = mountState()
    await flushPromises()
    await get().toggleArchivedSection()
    await get().selectItem(9)
    expect(get().selectedItem.value?.revision).toBe(2)
    await get().toggleArchive(get().selectedItem.value!)
    expect(mocks.ToggleArchived).toHaveBeenCalledWith(9, 2)
  })

  it('loads action runs by selected item id and does not let old action responses replace a new selection', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1), item(2)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    let resolve!: (value: { commandId: number; status: string }) => void
    mocks.InvokeAction.mockReturnValue(
      new Promise((r) => {
        resolve = r
      }),
    )
    const get = mountState()
    await flushPromises()
    await get().selectItem(1)
    const invocation = get().invokeAction('review')
    await get().selectItem(2)
    resolve({ commandId: 9, status: 'done' })
    await invocation
    expect(get().actionRuns.value).toEqual({})
  })

  it('loads actions by item id for webhook items now that the GitHub-only guard is gone', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(1, { sourceKind: 'webhook', sourceScope: 'hook-source', payload: { id: 'run-1', kind: 'deploy' } }),
    ])
    mocks.ActionViews.mockResolvedValue([
      { id: 'deploy', label: 'Deploy', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    const get = mountState()
    await flushPromises()
    expect(mocks.ActionViews).toHaveBeenCalledWith(1)
    expect(get().actions.value).toEqual([
      { id: 'deploy', label: 'Deploy', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
  })

  it('opens the selected item URL in the browser', async () => {
    mocks.ListByFeed.mockResolvedValue([item(3)])
    const get = mountState()
    await flushPromises()
    await get().openSelectedInBrowser()
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://example.test/3')
  })

  it('reconciles saved sidebar folders and persists reordered node ids', async () => {
    mocks.GetSidebar.mockResolvedValue({ items: [{ folder: { id: 'work', name: 'Work', feeds: ['my-prs'] } }] })
    const get = mountState()
    await flushPromises()
    expect(get().activeProfile.value?.tree).toMatchObject([
      { kind: 'folder', folder: { id: 'work', feeds: [{ id: 'triage/my-prs' }] } },
    ])
    await get().reorderFeeds('triage', [
      { kind: 'folder', folder: { id: 'work', name: 'Work', feeds: [get().activeProfile.value!.feeds[0]!] } },
    ])
    expect(mocks.SaveSidebar).toHaveBeenCalledWith('triage', {
      items: [{ folder: { id: 'work', name: 'Work', feeds: ['my-prs'] } }],
    })
  })

  it('toggles profile enablement while keeping its selection', async () => {
    const get = mountState()
    await flushPromises()
    expect(await get().setProfileEnabled('triage', false)).toBe(true)
    expect(mocks.SetFlowEnabled).toHaveBeenCalledWith('triage', false)
    expect(get().activeProfileId.value).toBe('triage')
    expect(get().activeProfile.value?.enabled).toBe(false)
    expect(mocks.notify).toHaveBeenCalledWith({
      title: 'Profile disabled',
      body: 'Frontend Triage',
      severity: 'success',
      category: 'config',
    })

    mocks.SetFlowEnabled.mockRejectedValueOnce(new Error('disk is read-only'))
    expect(await get().setProfileEnabled('triage', true)).toBe(false)
    expect(get().toggleProfileError.value).toBe('disk is read-only')
    expect(get().activeProfile.value?.enabled).toBe(false)
  })

  it('creates, renames, rejects failed renames, and deletes flow-backed profiles', async () => {
    mocks.CreateFlow.mockResolvedValue({ id: 'new', name: 'New', enabled: true, valid: true })
    mocks.RenameFlow.mockResolvedValue({ id: 'triage', name: 'Team Triage', enabled: true, valid: true })
    mocks.DeleteFlow.mockResolvedValue(undefined)
    const get = mountState()
    await flushPromises()
    await get().createProfile('New')
    expect(mocks.CreateFlow).toHaveBeenCalledWith('New')
    expect(get().profiles.value.some((profile) => profile.id === 'new')).toBe(true)
    await get().selectProfile('triage')
    expect(await get().renameProfile('triage', ' Team Triage ')).toBe(true)
    expect(get().activeProfile.value).toMatchObject({ name: 'Team Triage', letter: 'T' })
    expect(mocks.notify).toHaveBeenCalledWith({
      title: 'Profile renamed',
      body: 'Team Triage',
      severity: 'success',
      category: 'config',
    })
    mocks.RenameFlow.mockRejectedValueOnce(new Error('disk is read-only'))
    expect(await get().renameProfile('triage', 'Nope')).toBe(false)
    expect(get().renameProfileError.value).toBe('disk is read-only')
    await get().deleteProfile('triage')
    expect(mocks.DeleteFlow).toHaveBeenCalledWith('triage')
    expect(mocks.notify).toHaveBeenCalledWith({
      title: 'Profile deleted',
      body: 'Team Triage',
      severity: 'success',
      category: 'config',
    })
  })

  it('opens arbitrary URLs and reports a selected item without a URL', async () => {
    mocks.ListByFeed.mockResolvedValue([item(1, { url: '' })])
    const get = mountState()
    await flushPromises()
    await get().openSelectedInBrowser()
    expect(mocks.OpenURL).not.toHaveBeenCalled()
    expect(get().toasts.value.some((toast) => toast.message === 'No link available for this item')).toBe(true)
    await get().openUrl('https://example.test/docs')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://example.test/docs')
  })

  it('invokes actions by authoritative numeric item id and preserves success/failure feedback', async () => {
    mocks.ListByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    mocks.InvokeAction.mockResolvedValueOnce({ commandId: 17, status: 'done' })
    const get = mountState()
    await flushPromises()
    await get().invokeAction('review')
    expect(mocks.InvokeAction).toHaveBeenCalledWith('review', 7, {})
    expect(get().actionRuns.value.review?.commandId).toBe(17)
    expect(mocks.notify).toHaveBeenCalledWith({ title: 'Review completed', severity: 'success', category: 'action' })
    mocks.InvokeAction.mockResolvedValueOnce({
      commandId: 18,
      status: 'failed',
      error: 'command exited 1',
      stderr: 'bad input',
    })
    await get().invokeAction('review')
    expect(get().actionRuns.value.review).toMatchObject({ commandId: 18, status: 'failed', stderr: 'bad input' })
    expect(mocks.notify).toHaveBeenCalledWith({ title: 'command exited 1', severity: 'error', category: 'action' })
  })

  it('copies a clipboard action through the native clipboard with no durable run', async () => {
    mocks.ListByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([
      {
        id: 'copy-checkout',
        label: 'Copy checkout command',
        type: 'clipboard',
        showInDetail: true,
        requiresSessionInput: false,
      },
    ])
    mocks.RenderClipboardAction.mockResolvedValue('gh pr checkout 7 -R acme/app')
    const get = mountState()
    await flushPromises()
    await get().invokeAction('copy-checkout')
    expect(mocks.RenderClipboardAction).toHaveBeenCalledWith('copy-checkout', [7], {})
    expect(mocks.SetText).toHaveBeenCalledWith('gh pr checkout 7 -R acme/app')
    // A clipboard action never enqueues a durable command or records a run.
    expect(mocks.InvokeAction).not.toHaveBeenCalled()
    expect(get().actionRuns.value['copy-checkout']).toBeUndefined()
  })

  it('asks for confirmation before rerunning an action and preserves the first run', async () => {
    mocks.ListByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review PR', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    mocks.InvokeAction.mockResolvedValueOnce({
      commandId: 7,
      status: 'done',
      confirmationRequired: true,
    }).mockResolvedValueOnce({ commandId: 8, status: 'done' })
    const get = mountState()
    await flushPromises()
    await get().invokeAction('review')
    expect(get().actionRerunConfirmation.value).toMatchObject({ actionID: 'review', label: 'Review PR' })
    expect(mocks.notify).not.toHaveBeenCalled()
    await get().confirmActionRerun()
    expect(mocks.InvokeAction).toHaveBeenLastCalledWith('review', 7, { rerun: true })
    expect(get().actionRerunConfirmation.value).toBeNull()
    expect(mocks.notify).toHaveBeenCalledWith({ title: 'Review PR completed', severity: 'success', category: 'action' })
  })

  it('names published messages and opens interactive session launch actions before invocation', async () => {
    mocks.ListByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'notify', label: 'Notify', type: 'publish-message', showInDetail: true, requiresSessionInput: false },
    ])
    mocks.InvokeAction.mockResolvedValueOnce({
      commandId: 18,
      status: 'done',
      result: { message: { topic: 'agent.session.inbox', sender: 'hive-desktop' } },
    })
    const get = mountState()
    await flushPromises()
    await get().invokeAction('notify')
    expect(mocks.notify).toHaveBeenCalledWith({
      title: 'Published message to agent.session.inbox as hive-desktop',
      severity: 'success',
      category: 'action',
    })
    mocks.ActionViews.mockResolvedValue([
      { id: 'launch', label: 'Launch', type: 'launch-session', showInDetail: true, requiresSessionInput: true },
    ])
    await get().selectItem(7)
    await get().invokeAction('launch')
    expect(mocks.SessionLaunchOptions).toHaveBeenCalledOnce()
    expect(get().sessionLaunchAction.value?.id).toBe('launch')
    mocks.InvokeAction.mockResolvedValueOnce({
      commandId: 19,
      status: 'done',
      result: { session: { id: 'session-1', name: 'review-pr-7' } },
    })
    await get().submitSessionLaunch({
      name: 'review-pr-7',
      repository: 'https://github.com/hay-kot/hive-desktop.git',
      agent: 'claude',
      inputs: {},
    })
    expect(mocks.InvokeAction).toHaveBeenLastCalledWith('launch', 7, {
      session: { name: 'review-pr-7', repository: 'https://github.com/hay-kot/hive-desktop.git', agent: 'claude' },
      inputs: {},
    })
    expect(mocks.notify).toHaveBeenCalledWith({
      title: 'Created session review-pr-7 (session-1)',
      severity: 'success',
      category: 'session',
    })
  })

  it('collects declared inputs before invoking and sends them with the run', async () => {
    mocks.ListByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([
      {
        id: 'silence',
        label: 'Silence alert',
        type: 'shell',
        showInDetail: true,
        requiresSessionInput: false,
        inputs: [
          {
            name: 'reason',
            label: 'Reason',
            type: 'text',
            required: true,
            default: '',
            placeholder: '',
            options: null,
          },
        ],
      },
    ])
    const get = mountState()
    await flushPromises()

    await get().invokeAction('silence')
    expect(mocks.InvokeAction).not.toHaveBeenCalled()
    expect(get().actionInputsAction.value?.id).toBe('silence')

    mocks.InvokeAction.mockResolvedValueOnce({ commandId: 21, status: 'done' })
    await get().submitActionInputs({ reason: 'flapping' })
    expect(mocks.InvokeAction).toHaveBeenLastCalledWith('silence', 7, { inputs: { reason: 'flapping' } })
    expect(get().actionInputsAction.value).toBeNull()
  })

  it('routes a clipboard action with declared inputs through the render path', async () => {
    mocks.ListByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([
      {
        id: 'silence',
        label: 'Silence alert',
        type: 'clipboard',
        showInDetail: true,
        requiresSessionInput: false,
        inputs: [
          {
            name: 'reason',
            label: 'Reason',
            type: 'text',
            required: true,
            default: '',
            placeholder: '',
            options: null,
          },
        ],
      },
    ])
    mocks.RenderClipboardAction.mockResolvedValue('amtool silence add # flapping')
    const get = mountState()
    await flushPromises()

    await get().invokeAction('silence')
    expect(mocks.RenderClipboardAction).not.toHaveBeenCalled()
    await get().submitActionInputs({ reason: 'flapping' })
    expect(mocks.RenderClipboardAction).toHaveBeenCalledWith('silence', [7], { reason: 'flapping' })
    expect(mocks.SetText).toHaveBeenCalledWith('amtool silence add # flapping')
    expect(mocks.InvokeAction).not.toHaveBeenCalled()
  })

  // An interactive launch-session action can declare inputs too: one dialog
  // collects both, and both reach the invocation.
  it('sends declared inputs alongside session input from the launch dialog', async () => {
    mocks.ListByFeed.mockResolvedValue([item(7)])
    mocks.ActionViews.mockResolvedValue([
      {
        id: 'launch',
        label: 'Launch',
        type: 'launch-session',
        showInDetail: true,
        requiresSessionInput: true,
        inputs: [
          {
            name: 'focus',
            label: 'Focus',
            type: 'text',
            required: false,
            default: 'tests',
            placeholder: '',
            options: null,
          },
        ],
      },
    ])
    const get = mountState()
    await flushPromises()

    await get().invokeAction('launch')
    expect(get().actionInputsAction.value).toBeNull()
    expect(get().sessionLaunchAction.value?.id).toBe('launch')

    mocks.InvokeAction.mockResolvedValueOnce({
      commandId: 22,
      status: 'done',
      result: { session: { id: 's1', name: 'n' } },
    })
    await get().submitSessionLaunch({ name: 'n', repository: 'r', inputs: { focus: 'tests' } })
    expect(mocks.InvokeAction).toHaveBeenLastCalledWith('launch', 7, {
      session: { name: 'n', repository: 'r' },
      inputs: { focus: 'tests' },
    })
  })

  it('scopes persisted action runs to the numeric item that owns them', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ '1': { review: 41 } }))
    mocks.ListByFeed.mockResolvedValue([item(1), item(2)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    mocks.ActionRun.mockResolvedValue({ commandId: 41, status: 'failed', stderr: 'details' })
    const get = mountState()
    await flushPromises()
    expect(mocks.ActionRun).toHaveBeenCalledWith(41)
    expect(get().actionRuns.value.review?.stderr).toBe('details')
    await get().selectItem(2)
    await flushPromises()
    expect(get().actionRuns.value.review).toBeUndefined()
    await get().selectItem(1)
    await flushPromises()
    expect(mocks.ActionRun).toHaveBeenCalledTimes(2)
  })

  // The Kind the Go core assigns is what decides whether a stale run id is
  // dropped. Matching on the message used to do this by accident: any error
  // whose text happened to contain "missing" silently forgot a live run.
  function bindingError(kind: string): Error {
    const error = new Error('a bound method returned an error')
    ;(error as Error & { cause?: unknown }).cause = { kind, message: 'action run 41 not found' }
    return error
  }

  it('drops a stale action run id when the core says not_found', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ '1': { review: 41 } }))
    mocks.ListByFeed.mockResolvedValue([item(1)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    mocks.ActionRun.mockRejectedValue(bindingError('not_found'))

    const get = mountState()
    await flushPromises()

    expect(get().actionRuns.value.review).toBeUndefined()
    expect(JSON.parse(localStorage.getItem('hive.action-run-ids') ?? '{}')).toEqual({})
  })

  it('keeps a run id when the failure is not a missing row', async () => {
    for (const kind of ['internal', 'unavailable', 'unauthenticated']) {
      localStorage.setItem('hive.action-run-ids', JSON.stringify({ '1': { review: 41 } }))
      mocks.ListByFeed.mockResolvedValue([item(1)])
      mocks.ActionViews.mockResolvedValue([
        { id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false },
      ])
      mocks.ActionRun.mockRejectedValue(bindingError(kind))

      const get = mountState()
      await flushPromises()

      expect(JSON.parse(localStorage.getItem('hive.action-run-ids') ?? '{}'), kind).toEqual({ '1': { review: 41 } })
      expect(get().actionRuns.value.review, kind).toBeUndefined()
    }
  })

  it('keeps a run id when the failure carries no kind at all', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ '1': { review: 41 } }))
    mocks.ListByFeed.mockResolvedValue([item(1)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    mocks.ActionRun.mockRejectedValue(new Error('the item was not found'))

    mountState()
    await flushPromises()

    // The old regex matched this message and forgot the run. A message is
    // not a contract.
    expect(JSON.parse(localStorage.getItem('hive.action-run-ids') ?? '{}')).toEqual({ '1': { review: 41 } })
  })

  it('rejects malformed persisted action run ids without restoring them', async () => {
    localStorage.setItem('hive.action-run-ids', JSON.stringify({ '1': { review: '41', zero: 0 }, bad: [] }))
    mocks.ListByFeed.mockResolvedValue([item(1)])
    mocks.ActionViews.mockResolvedValue([
      { id: 'review', label: 'Review', type: 'shell', showInDetail: true, requiresSessionInput: false },
    ])
    const get = mountState()
    await flushPromises()
    expect(get().actionRuns.value).toEqual({})
    expect(mocks.ActionRun).not.toHaveBeenCalled()
  })

  it('keeps the newer list when an older selection request resolves late', async () => {
    const get = mountState()
    await flushPromises()
    const resolveFeed: Array<(rows: InboxItem[]) => void> = []
    const resolveTrash: Array<(rows: InboxItem[]) => void> = []
    mocks.ListByFeed.mockImplementation(() => new Promise<InboxItem[]>((done) => resolveFeed.push(done)))
    mocks.ListTrash.mockImplementation(() => new Promise<InboxItem[]>((done) => resolveTrash.push(done)))
    const feed = get().selectSidebar({ type: 'feed', feedId: 'triage/my-prs' })
    await flushPromises()
    const trash = get().selectSidebar({ type: 'trash' })
    await flushPromises()
    resolveTrash[0]!([item(2, { ignoredAt: 2 })])
    await trash
    resolveFeed[0]!([item(1)])
    await feed
    expect(get().selection.value).toEqual({ type: 'trash' })
    expect(get().items.value.map((row) => row.id)).toEqual([2])
  })

  it('renders observed events oldest-to-newest even though the storage read is newest-first', async () => {
    mocks.Events.mockResolvedValue([
      { id: 3, itemId: 1, kind: 'closed', transition: 'closed', attention: 'none', summary: 'newest', createdAt: 3 },
      {
        id: 2,
        itemId: 1,
        kind: 'updated',
        transition: 'updated',
        attention: 'activity',
        summary: 'middle',
        createdAt: 2,
      },
      {
        id: 1,
        itemId: 1,
        kind: 'created',
        transition: 'created',
        attention: 'activity',
        summary: 'oldest',
        createdAt: 1,
      },
    ])
    const get = mountState()
    await flushPromises()
    expect((await get().loadEvents(1)).map((event) => event.summary)).toEqual(['oldest', 'middle', 'newest'])
  })

  it('retains load errors, clears stale rows, and retries the current selection on refresh', async () => {
    mocks.ListByFeed.mockRejectedValueOnce(new Error('offline'))
    const get = mountState()
    await flushPromises()
    expect(get().loadError.value).toBe("Can't load inbox items right now.")
    expect(get().items.value).toEqual([])
    mocks.ListTrash.mockResolvedValue([item(9, { ignoredAt: 9 })])
    await get().selectSidebar({ type: 'trash' })
    expect(get().items.value[0]).toMatchObject({ id: 9 })
    await get().refresh()
    expect(mocks.ListTrash).toHaveBeenLastCalledWith('triage', 500)
  })

  it('fetches from the sources on a manual refresh and never on the passive one', async () => {
    const get = mountState()
    await flushPromises()
    await get().refresh()
    expect(mocks.RefreshSources).not.toHaveBeenCalled()

    await get().refreshSources()
    expect(mocks.RefreshSources).toHaveBeenCalledTimes(1)
    expect(get().refreshingSources.value).toBe(false)
  })

  it('coalesces manual refreshes while one is in flight', async () => {
    let release!: () => void
    mocks.RefreshSources.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        release = () => resolve()
      }),
    )
    const get = mountState()
    await flushPromises()

    const first = get().refreshSources()
    expect(get().refreshingSources.value).toBe(true)
    await get().refreshSources()
    expect(mocks.RefreshSources).toHaveBeenCalledTimes(1)

    release()
    await first
    expect(get().refreshingSources.value).toBe(false)
  })

  it('still reloads when a source fetch fails, and stays quiet when there is nothing to fetch', async () => {
    const get = mountState()
    await flushPromises()

    mocks.RefreshSources.mockRejectedValueOnce(
      Object.assign(new Error('call failed'), {
        cause: { kind: 'unavailable', message: 'source refresh is unavailable in this mode' },
      }),
    )
    await get().refreshSources()
    expect(mocks.notify).not.toHaveBeenCalled()

    mocks.RefreshSources.mockRejectedValueOnce(
      Object.assign(new Error('call failed'), { cause: { kind: 'internal', message: 'grafana: 502' } }),
    )
    mocks.ListTrash.mockClear()
    await get().selectSidebar({ type: 'trash' })
    mocks.ListTrash.mockClear()
    await get().refreshSources()
    expect(mocks.notify).toHaveBeenCalledWith(expect.objectContaining({ title: 'grafana: 502', severity: 'error' }))
    expect(mocks.ListTrash).toHaveBeenCalled()
  })

  it('navigates only across the searched unread subset as selected rows become read', async () => {
    mocks.ListByFeed.mockResolvedValue([
      item(3, { title: 'Onboard', unread: true }),
      item(2, { title: 'Deploy', unread: true }),
      item(1, { title: 'Onload', unread: true }),
    ])
    const get = mountState()
    await flushPromises()
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
