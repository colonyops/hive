import { describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import App from '../App.vue'
import { useCommandPalette } from '../composables/useCommands'
import { resetFlowsSessionForTests, useFlowsSession } from '../pipeline/composables/useFlowsSession'
import { createAppRouter } from '../router'

const mocks = vi.hoisted(() => ({
  // flowsservice
  ListFlows: vi.fn(),
  GetFlow: vi.fn(),
  CreateFlow: vi.fn(),
  RenameFlow: vi.fn(),
  SetFlowEnabled: vi.fn(),
  DeleteFlow: vi.fn(),
  GetLayout: vi.fn(),
  SaveFlow: vi.fn(),
  SaveLayout: vi.fn(),
  GetSidebar: vi.fn(),
  SaveSidebar: vi.fn(),
  // actionsservice
  ListActions: vi.fn(),
  CreateAction: vi.fn(),
  UpdateAction: vi.fn(),
  DeleteAction: vi.fn(),
  // pipelineservice
  ListInboxItems: vi.fn(),
  ListInboxItemsByFeed: vi.fn(),
  FeedCounts: vi.fn(),
  InboxCounts: vi.fn(),
  MarkInboxItemUnread: vi.fn(),
  ToggleInboxItemArchived: vi.fn(),
  ToggleInboxItemIgnored: vi.fn(),
  InboxItemEvents: vi.fn(),
  ActionRun: vi.fn(),
  SessionLaunchOptions: vi.fn(),
  EventLogTailOffset: vi.fn(),
  ActivateReplay: vi.fn(),
  ListUnarchivedInboxItems: vi.fn(),
  ListReplaySourceSnapshots: vi.fn(),
  ActionViews: vi.fn(),
  InvokeAction: vi.fn(),
  NodeRuns: vi.fn(),
  ReadFrom: vi.fn(),
  Commit: vi.fn(),
  // auth service
  Status: vi.fn(),
  StartDeviceFlow: vi.fn(),
  CancelDeviceFlow: vi.fn(),
  SetToken: vi.fn(),
  SignOut: vi.fn(),
  // updaterservice
  UpdaterStatus: vi.fn(),
  InstallUpdate: vi.fn(),
  // runtime
  On: vi.fn(),
  Hide: vi.fn(),
}))

vi.mock('../../bindings/github.com/colonyops/hive/desktop/flowsservice', () => ({
  ListFlows: mocks.ListFlows,
  GetFlow: mocks.GetFlow,
  CreateFlow: mocks.CreateFlow,
  RenameFlow: mocks.RenameFlow,
  SetFlowEnabled: mocks.SetFlowEnabled,
  DeleteFlow: mocks.DeleteFlow,
  GetLayout: mocks.GetLayout,
  SaveFlow: mocks.SaveFlow,
  SaveLayout: mocks.SaveLayout,
  GetSidebar: mocks.GetSidebar,
  SaveSidebar: mocks.SaveSidebar,
}))

vi.mock('../../bindings/github.com/colonyops/hive/desktop/actionsservice', () => ({
  ListActions: mocks.ListActions,
  CreateAction: mocks.CreateAction,
  UpdateAction: mocks.UpdateAction,
  DeleteAction: mocks.DeleteAction,
}))

vi.mock('../../bindings/github.com/colonyops/hive/desktop/pipelineservice', () => ({
  ListInboxItems: mocks.ListInboxItems,
  ListInboxItemsByFeed: mocks.ListInboxItemsByFeed,
  FeedCounts: mocks.FeedCounts,
  InboxCounts: mocks.InboxCounts,
  MarkInboxItemUnread: mocks.MarkInboxItemUnread,
  ToggleInboxItemArchived: mocks.ToggleInboxItemArchived,
  ToggleInboxItemIgnored: mocks.ToggleInboxItemIgnored,
  InboxItemEvents: mocks.InboxItemEvents,
  ActionRun: mocks.ActionRun,
  SessionLaunchOptions: mocks.SessionLaunchOptions,
  EventLogTailOffset: mocks.EventLogTailOffset,
  ActivateReplay: mocks.ActivateReplay,
  ListUnarchivedInboxItems: mocks.ListUnarchivedInboxItems,
  ListReplaySourceSnapshots: mocks.ListReplaySourceSnapshots,
  ActionViews: mocks.ActionViews,
  InvokeAction: mocks.InvokeAction,
  NodeRuns: mocks.NodeRuns,
  ReadFrom: mocks.ReadFrom,
  Commit: mocks.Commit,
}))

vi.mock('../../bindings/github.com/colonyops/hive/internal/desktop/auth/service', () => ({
  Status: mocks.Status,
  StartDeviceFlow: mocks.StartDeviceFlow,
  CancelDeviceFlow: mocks.CancelDeviceFlow,
  SetToken: mocks.SetToken,
  SignOut: mocks.SignOut,
}))

vi.mock('../../bindings/github.com/colonyops/hive/desktop/updaterservice', () => ({
  Status: mocks.UpdaterStatus,
  InstallUpdate: mocks.InstallUpdate,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Window: { Hide: mocks.Hide },
  Call: { ByID: vi.fn() },
}))

const flow = {
  id: 'personal',
  name: 'Personal',
  enabled: true,
  nodes: [
    { id: 'src', type: 'github-source' },
    { id: 'desktop', type: 'feed', name: 'Desktop UI' },
  ],
  wires: [{ from: 'src', to: 'desktop' }],
}

async function mountAppWithRouter() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/')
  await router.isReady()
  const wrapper = mount(App, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

async function mountApp() {
  return (await mountAppWithRouter()).wrapper
}

describe('App', () => {
  beforeEach(() => {
    // useFlowsSession is a module singleton (App.vue + FlowsView.vue share
    // one instance) — without this, a later test would silently reuse a
    // prior test's instance, including its already-torn-down onMounted/
    // watch hooks from that test's wrapper.unmount().
    resetFlowsSessionForTests()
    vi.clearAllMocks()
    // Panel collapse / width state persists via useStorage; clear it so one
    // test's collapsed sidebar can't leak into the next.
    localStorage.clear()
    mocks.Status.mockResolvedValue({ state: 'authenticated', login: 'hay', name: 'Hay', avatarUrl: '', message: '' })
    mocks.ListFlows.mockResolvedValue([{ id: 'personal', name: 'Personal', enabled: true, valid: true }])
    mocks.GetFlow.mockResolvedValue(flow)
    mocks.GetLayout.mockResolvedValue({ nodes: {} })
    mocks.GetSidebar.mockResolvedValue({ items: [] })
    mocks.SaveSidebar.mockResolvedValue(undefined)
    mocks.ListInboxItems.mockResolvedValue([])
    mocks.ListInboxItemsByFeed.mockResolvedValue([])
    mocks.FeedCounts.mockResolvedValue([{ feedId: 'personal/desktop', total: 1, unread: 0 }])
    mocks.InboxCounts.mockResolvedValue({ inboxTotal: 1, inboxUnread: 0 })
    mocks.InboxItemEvents.mockResolvedValue([])
    mocks.ActionRun.mockResolvedValue({ commandId: 1, status: 'done' })
    mocks.SessionLaunchOptions.mockResolvedValue({ repositories: [], defaultRepository: '', agents: [], defaultAgent: '' })
    mocks.EventLogTailOffset.mockResolvedValue('0')
    mocks.ActivateReplay.mockResolvedValue(undefined)
    mocks.ListUnarchivedInboxItems.mockResolvedValue([])
    mocks.ListReplaySourceSnapshots.mockResolvedValue([])
    mocks.ActionViews.mockResolvedValue([])
    mocks.InvokeAction.mockResolvedValue(undefined)
    mocks.ListActions.mockResolvedValue({ actions: [], error: '' })
    mocks.NodeRuns.mockResolvedValue([])
    mocks.RenameFlow.mockResolvedValue({ id: 'personal', name: 'Team', enabled: true, valid: true })
    mocks.SetFlowEnabled.mockImplementation(async (id: string, enabled: boolean) => ({ id, name: 'Personal', enabled, valid: true }))
    mocks.DeleteFlow.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
    mocks.UpdaterStatus.mockResolvedValue({ enabled: true, available: false, currentVersion: 'dev', latestVersion: '', notes: '', releaseUrl: '' })
    mocks.InstallUpdate.mockResolvedValue(undefined)
  })

  it('registers profile / feed-selection / flow-edit palette commands (not the removed feed-editor ones)', async () => {
    const wrapper = await mountApp()
    const { results, query } = useCommandPalette()
    query.value = ''

    const ids = results.value.map((cmd) => cmd.id)
    expect(ids).toContain('flow:edit')
    expect(ids).toContain('feed:all')
    expect(ids).toContain('feed:personal/desktop')
    expect(ids).toContain('profile:new')
    // Feed/source editing folded into the node drawer — these are gone.
    expect(ids).not.toContain('feed:new')
    expect(ids).not.toContain('feed:edit:desktop')
    expect(ids).not.toContain('feed:edit-config')

    wrapper.unmount()
  })

  it('opens the flows canvas from the sidebar and exits via the profile rail, keeping the rail mounted', async () => {
    const wrapper = await mountApp()

    // Feed view first: sidebar present, no flows canvas.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    // Flows canvas is up; the spaces rail stays mounted as the way back.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(true)

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    // Back to the feed view.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('renames the active profile from profile settings', async () => {
    const wrapper = await mountApp()

    await wrapper.get('[data-testid="sidebar-open-settings"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="profile-settings-name"]').setValue('Team')
    await wrapper.get('[data-testid="profile-settings-view"] form').trigger('submit')
    await flushPromises()

    expect(mocks.RenameFlow).toHaveBeenCalledWith('personal', 'Team')
    expect((wrapper.get('[data-testid="profile-settings-name"]').element as HTMLInputElement).value).toBe('Team')

    wrapper.unmount()
  })

  it('collapses and restores the feed sidebar from the title-bar toggle', async () => {
    const wrapper = await mountApp()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    await wrapper.find('[data-testid="titlebar-toggle-sidebar"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(false)

    await wrapper.find('[data-testid="titlebar-toggle-sidebar"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('collapses and restores the detail preview from the title-bar toggle and the p key', async () => {
    const wrapper = await mountApp()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(true)

    await wrapper.find('[data-testid="titlebar-toggle-preview"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(false)
    expect(localStorage.getItem('hive.panel.detailpane.collapsed')).toBe('true')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'p' }))
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-pane"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('opens profile settings from the sidebar gear and application settings from the rail', async () => {
    const wrapper = await mountApp()

    await wrapper.find('[data-testid="sidebar-open-settings"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="profile-settings-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(false)

    await wrapper.find('[data-testid="profile-settings-close"]').trigger('click')
    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="settings-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profile-settings-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(true)

    await wrapper.find('[data-testid="settings-close"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('uses route history for settings pages and categories', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')
    expect(wrapper.find('[data-testid="settings-theme-toggle-dark"]').exists()).toBe(true)

    await wrapper.find('[data-testid="settings-category-integrations"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('integrations')
    expect(wrapper.find('[data-testid="settings-integrations"]').exists()).toBe(true)

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')
    expect(router.currentRoute.value.params.section).toBe('')
    expect(wrapper.find('[data-testid="settings-theme-toggle-dark"]').exists()).toBe(true)

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('feed')
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('uses mouse back and forward buttons for route history', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('application-settings')

    const backDown = new MouseEvent('mousedown', { button: 3, cancelable: true })
    window.dispatchEvent(backDown)
    expect(backDown.defaultPrevented).toBe(true)

    const backUp = new MouseEvent('mouseup', { button: 3, cancelable: true })
    window.dispatchEvent(backUp)
    await flushPromises()
    expect(backUp.defaultPrevented).toBe(true)
    expect(router.currentRoute.value.name).toBe('feed')

    const forwardUp = new MouseEvent('mouseup', { button: 4, cancelable: true })
    window.dispatchEvent(forwardUp)
    await flushPromises()
    expect(forwardUp.defaultPrevented).toBe(true)
    expect(router.currentRoute.value.name).toBe('application-settings')

    wrapper.unmount()
  })

  it('suppresses Backspace history navigation outside editable fields', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    await flushPromises()
    const routeBefore = router.currentRoute.value.fullPath

    const backspace = new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true, cancelable: true })
    window.dispatchEvent(backspace)
    await flushPromises()

    expect(backspace.defaultPrevented).toBe(true)
    expect(router.currentRoute.value.fullPath).toBe(routeBefore)

    wrapper.unmount()
  })

  it('allows Backspace to edit text inputs', async () => {
    const { wrapper } = await mountAppWithRouter()
    const search = wrapper.get('[data-testid="feed-search"]').element
    const backspace = new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true, cancelable: true })

    search.dispatchEvent(backspace)

    expect(backspace.defaultPrevented).toBe(false)
    wrapper.unmount()
  })

  it('routes DetailPane Edit to actions settings', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/')
    await router.isReady()
    const wrapper = mount(App, {
      global: {
        plugins: [router],
        stubs: { DetailPane: { template: '<button data-testid="detail-edit" @click="$emit(\'edit\')" />', emits: ['edit'] } },
      },
    })
    await flushPromises()
    await wrapper.get('[data-testid="detail-edit"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value).toMatchObject({ name: 'application-settings', params: { section: 'actions' } })
    expect(wrapper.find('[data-testid="actions-settings"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders the actions settings deep-link and preserves it through back/forward history', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/settings/actions')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('actions')
    expect(wrapper.find('[data-testid="actions-settings"]').exists()).toBe(true)

    await router.push('/settings/integrations')
    await flushPromises()
    router.back()
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('actions')
    expect(wrapper.find('[data-testid="actions-settings"]').exists()).toBe(true)
    router.forward()
    await flushPromises()
    expect(router.currentRoute.value.params.section).toBe('integrations')
    wrapper.unmount()
  })

  it('records feed and unread navigation in back/forward history', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.find('[data-testid="sidebar-feed"][data-id="personal/desktop"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.feed).toBe('personal/desktop')
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="personal/desktop"]').classes()).toContain('sidebar-entry-selected')

    await wrapper.find('[data-testid="filter-unread"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ feed: 'personal/desktop', unread: '1' })

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ feed: 'personal/desktop' })
    expect(wrapper.find('[data-testid="filter-all"]').classes()).toContain('active')

    router.back()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})
    expect(wrapper.find('[data-testid="inbox-view-inbox"]').classes()).toContain('sidebar-entry-selected')

    router.forward()
    await flushPromises()
    expect(router.currentRoute.value.query.feed).toBe('personal/desktop')

    wrapper.unmount()
  })

  it('preserves a non-default inbox view when toggling unread', async () => {
    const { wrapper, router } = await mountAppWithRouter()

    await wrapper.get('[data-testid="inbox-view-archive"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ view: 'archive' })

    await wrapper.get('[data-testid="filter-unread"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ view: 'archive', unread: '1' })
    expect(mocks.ListInboxItems).toHaveBeenLastCalledWith('personal', 'archive', 500)
    expect(wrapper.get('[data-testid="inbox-view-archive"]').classes()).toContain('sidebar-entry-selected')
    wrapper.unmount()
  })

  it('clears stale observed activity while the selected item timeline loads or fails', async () => {
    const items = [
      { id: 1, profileId: 'personal', sourceKind: 'github', sourceScope: '', externalId: 'pr-1', title: 'First', url: '', payload: { kind: 'PR', repo: 'acme/app', num: 1, author: 'hay' }, revision: 1, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: 2 },
      { id: 2, profileId: 'personal', sourceKind: 'github', sourceScope: '', externalId: 'pr-2', title: 'Second', url: '', payload: { kind: 'PR', repo: 'acme/app', num: 2, author: 'hay' }, revision: 1, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: 2 },
    ]
    let rejectSecond!: (error: Error) => void
    const secondEvents = new Promise<never>((_, reject) => { rejectSecond = reject })
    mocks.ListInboxItems.mockResolvedValue(items)
    mocks.InboxItemEvents.mockImplementation((id: number) => id === 1
      ? Promise.resolve([{ id: 1, itemId: 1, kind: 'observed', transition: 'none', attention: 'activity', summary: 'first event', detail: {}, createdAt: 1 }])
      : secondEvents)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const wrapper = await mountApp()
    expect(wrapper.get('[data-testid="observed-activity"]').text()).toContain('first event')

    await wrapper.findAll('[data-testid="feed-item"]')[1]!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="observed-activity"]').exists()).toBe(false)

    rejectSecond(new Error('events unavailable'))
    await flushPromises()
    expect(wrapper.find('[data-testid="observed-activity"]').exists()).toBe(false)
    expect(warn).toHaveBeenCalledWith('Unable to load inbox item events', expect.any(Error))
    wrapper.unmount()
  })

  it('guards native back navigation when the flow has un-deployed changes', async () => {
    const { wrapper, router } = await mountAppWithRouter()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')
    router.back()
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('flows')
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-discard"]')?.click()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('feed')
    expect(session.dirty.value).toBe(false)

    wrapper.unmount()
  })

  it('deletes the active profile from profile settings, then falls back to onboarding', async () => {
    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="sidebar-delete-profile"]').exists()).toBe(false)
    await wrapper.find('[data-testid="sidebar-open-settings"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="profile-settings-danger"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="profile-settings-delete"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-testid="delete-profile-modal"]')).not.toBeNull()

    mocks.ListFlows.mockResolvedValue([])
    document.querySelector<HTMLButtonElement>('[data-testid="delete-profile-confirm"]')?.click()
    await flushPromises()

    expect(mocks.DeleteFlow).toHaveBeenCalledWith('personal')
    expect(document.querySelector('[data-testid="delete-profile-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('binds the active profile draft even with the flows canvas closed (hc-8ft4yhm6)', async () => {
    const wrapper = await mountApp()

    // GetLayout/NodeRuns are only ever called from usePipelineEditor's
    // selectFlow — never from useFeedState — so seeing them here proves
    // the app-wide session selected and loaded the active profile draft even
    // though the flows canvas was never opened.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(mocks.GetLayout).toHaveBeenCalledWith('personal')
    expect(mocks.NodeRuns).toHaveBeenCalledWith('personal', 100)

    wrapper.unmount()
  })

  it('reconciles deployed runtimes when flows:updated arrives with the canvas closed', async () => {
    const wrapper = await mountApp()
    const getFlowCalls = mocks.GetFlow.mock.calls.length
    const handler = mocks.On.mock.calls.find(([event]) => event === 'flows:updated')?.[1] as (() => void) | undefined

    expect(handler).toBeDefined()
    handler?.()
    await vi.waitFor(() => expect(mocks.GetFlow.mock.calls.length).toBeGreaterThan(getFlowCalls))

    wrapper.unmount()
  })

  it('the titlebar error chip deep-links to the first failing node, even with the canvas closed', async () => {
    mocks.NodeRuns.mockResolvedValue([
      { flowId: 'personal', nodeId: 'src', ok: false, inCount: 0, outCount: 0, dropCount: 0, err: 'boom', durMs: 1, endedAt: 0 },
    ])

    const wrapper = await mountApp()

    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    const chip = wrapper.find('[data-testid="titlebar-error-chip"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('1 error')

    await chip.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)
    const session = useFlowsSession()
    expect(session.flowFocusNodeId.value).toBe('src')

    wrapper.unmount()
  })

  it('registers a ⌘K "jump to node" command per node in the active flow', async () => {
    const wrapper = await mountApp()
    const { results, query } = useCommandPalette()
    query.value = ''

    const ids = results.value.map((cmd) => cmd.id)
    expect(ids).toContain('flow:node:src')
    expect(ids).toContain('flow:node:desktop')

    const nodeCmd = results.value.find((cmd) => cmd.id === 'flow:node:desktop')
    expect(nodeCmd?.title).toBe('Jump to node: Desktop UI')

    nodeCmd?.run()
    await flushPromises()
    expect(useFlowsSession().flowFocusNodeId.value).toBe('desktop')

    wrapper.unmount()
  })

  // ── Un-deployed changes guard (hc-sx4k3c7k) ──────────────────────────────

  it('shows the un-deployed changes badge in the sidebar once the flow is dirty', async () => {
    const wrapper = await mountApp()
    expect(wrapper.find('[data-testid="undeployed-badge"]').exists()).toBe(false)

    useFlowsSession().addNode('feed')
    await flushPromises()

    expect(wrapper.find('[data-testid="undeployed-badge"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('exiting the canvas via the profile rail while dirty prompts a confirm instead of leaving immediately; Cancel stays in the canvas', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)

    const session = useFlowsSession()
    session.addNode('feed')
    expect(session.dirty.value).toBe(true)

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    // Still in the canvas — the exit was deferred behind the confirm modal.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-cancel"]')?.click()
    await flushPromises()

    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true) // cancel aborted the exit
    expect(session.dirty.value).toBe(true) // draft untouched

    wrapper.unmount()
  })

  it('exiting the canvas via the profile rail while dirty: Deploy saves the draft then returns to the feed view', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-deploy"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).toHaveBeenCalled()
    expect(mocks.SaveLayout).toHaveBeenCalled()
    expect(session.dirty.value).toBe(false)
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('exiting the canvas via the profile rail while dirty: Discard drops the draft (reloads from disk) then returns to the feed view', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')
    const getFlowCallsBefore = mocks.GetFlow.mock.calls.length

    await wrapper.find('[data-testid="profile-tile"][data-id="personal"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-discard"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).not.toHaveBeenCalled()
    expect(mocks.GetFlow.mock.calls.length).toBeGreaterThan(getFlowCallsBefore) // discard reloaded from disk
    expect(session.dirty.value).toBe(false)
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('switching profiles from the rail while dirty prompts a confirm instead of switching immediately; Cancel stays on the current profile', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    const session = useFlowsSession()
    session.addNode('feed') // dirties the active ("personal") flow's draft
    expect(session.dirty.value).toBe(true)

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    // Still on the personal profile — the switch was deferred behind the confirm modal.
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Personal')
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-cancel"]')?.click()
    await flushPromises()

    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Personal') // cancel aborted the switch
    expect(session.dirty.value).toBe(true) // draft untouched
    expect(mocks.GetLayout).not.toHaveBeenCalledWith('work')

    wrapper.unmount()
  })

  it('switching profiles from the rail while dirty: Deploy saves the draft then switches profiles', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    const session = useFlowsSession()
    session.addNode('feed')

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-deploy"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).toHaveBeenCalled()
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Work')
    expect(mocks.GetLayout).toHaveBeenCalledWith('work')

    wrapper.unmount()
  })

  it('switching profiles from the rail while dirty: Discard drops the draft then switches profiles', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    const session = useFlowsSession()
    session.addNode('feed')

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="unsaved-flow-discard"]')?.click()
    await flushPromises()

    expect(mocks.SaveFlow).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Work')
    expect(mocks.GetLayout).toHaveBeenCalledWith('work')

    wrapper.unmount()
  })

  it('switching profiles from the rail is instant (no confirm) when the flow is not dirty', async () => {
    mocks.ListFlows.mockResolvedValue([
      { id: 'personal', name: 'Personal', enabled: true, valid: true },
      { id: 'work', name: 'Work', enabled: true, valid: true },
    ])
    mocks.GetFlow.mockImplementation(async (id: string) =>
      id === 'work' ? { id: 'work', name: 'Work', enabled: true, nodes: [], wires: [] } : flow,
    )

    const wrapper = await mountApp()
    expect(useFlowsSession().dirty.value).toBe(false)

    await wrapper.find('[data-testid="profile-tile"][data-id="work"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="unsaved-flow-changes-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="sidebar-profile-name"]').text()).toBe('Work')

    wrapper.unmount()
  })

  it('on "log:appended", pumps the runtime (commit) BEFORE refreshing the feed — the commit must land before the re-read', async () => {
    const wrapper = await mountApp()

    const callOrder: string[] = []
    mocks.ReadFrom.mockResolvedValueOnce([{ ID: '1', Key: '1', Topic: 'source:personal/src', Ts: 0, Payload: {}, SourceKind: 'github', SourceScope: 'src' }])
    mocks.Commit.mockImplementationOnce(async () => { callOrder.push('commit') })
    mocks.InboxCounts.mockImplementationOnce(async () => { callOrder.push('refresh'); return { inboxTotal: 0, inboxUnread: 0 } })

    const logHandler = mocks.On.mock.calls.find(([event]) => event === 'log:appended')?.[1] as (() => void) | undefined
    expect(logHandler).toBeDefined()

    logHandler?.()
    // The drain yields between pages before its terminating empty read.
    await vi.waitFor(() => expect(callOrder).toEqual(['commit', 'refresh']))

    wrapper.unmount()
  })
})
