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
  FeedItems: vi.fn(),
  FeedItemCounts: vi.fn(),
  MarkFeedItemRead: vi.fn(),
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
  // runtime
  On: vi.fn(),
  Hide: vi.fn(),
}))

vi.mock('../../bindings/github.com/colonyops/hive/desktop/flowsservice', () => ({
  ListFlows: mocks.ListFlows,
  GetFlow: mocks.GetFlow,
  CreateFlow: mocks.CreateFlow,
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
  FeedItems: mocks.FeedItems,
  FeedItemCounts: mocks.FeedItemCounts,
  MarkFeedItemRead: mocks.MarkFeedItemRead,
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

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Window: { Hide: mocks.Hide },
}))

const flow = {
  id: 'personal',
  name: 'Personal',
  enabled: true,
  nodes: [
    { id: 'src', type: 'github-source' },
    { id: 'desktop', type: 'feed', name: 'Desktop UI' },
  ],
  wires: [],
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
    mocks.Status.mockResolvedValue({ state: 'authenticated', login: 'hay', name: 'Hay', avatarUrl: '', message: '' })
    mocks.ListFlows.mockResolvedValue([{ id: 'personal', name: 'Personal', enabled: true, valid: true }])
    mocks.GetFlow.mockResolvedValue(flow)
    mocks.GetLayout.mockResolvedValue({ nodes: {} })
    mocks.GetSidebar.mockResolvedValue({ items: [] })
    mocks.SaveSidebar.mockResolvedValue(undefined)
    mocks.FeedItems.mockResolvedValue([])
    mocks.FeedItemCounts.mockResolvedValue([{ feedId: 'personal/desktop', total: 1, unread: 0 }])
    mocks.ActionViews.mockResolvedValue([])
    mocks.InvokeAction.mockResolvedValue(undefined)
    mocks.ListActions.mockResolvedValue({ actions: [], error: '' })
    mocks.NodeRuns.mockResolvedValue([])
    mocks.DeleteFlow.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
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

  it('opens the flows canvas from the sidebar and exits via the breadcrumb, keeping the rail mounted', async () => {
    const wrapper = await mountApp()

    // Feed view first: sidebar present, no flows canvas.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    // Flows canvas is up; the spaces rail stays; the breadcrumb offers a way back.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profile-tile"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="breadcrumb-flows"]').exists()).toBe(true)

    await wrapper.find('[data-testid="breadcrumb-profile-name"]').trigger('click')
    await flushPromises()

    // Back to the feed view.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').exists()).toBe(true)

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
    const allItems = wrapper.findAll('button.sidebar-entry').find((button) => button.text().includes('All items'))
    expect(allItems?.classes()).toContain('sidebar-entry-selected')

    router.forward()
    await flushPromises()
    expect(router.currentRoute.value.query.feed).toBe('personal/desktop')

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

  it('exiting the canvas via the breadcrumb while dirty prompts a confirm instead of leaving immediately; Cancel stays in the canvas', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(true)

    const session = useFlowsSession()
    session.addNode('feed')
    expect(session.dirty.value).toBe(true)

    await wrapper.find('[data-testid="breadcrumb-profile-name"]').trigger('click')
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

  it('exiting the canvas via the breadcrumb while dirty: Deploy saves the draft then returns to the feed view', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')

    await wrapper.find('[data-testid="breadcrumb-profile-name"]').trigger('click')
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

  it('exiting the canvas via the breadcrumb while dirty: Discard drops the draft (reloads from disk) then returns to the feed view', async () => {
    const wrapper = await mountApp()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    await flushPromises()

    const session = useFlowsSession()
    session.addNode('feed')
    const getFlowCallsBefore = mocks.GetFlow.mock.calls.length

    await wrapper.find('[data-testid="breadcrumb-profile-name"]').trigger('click')
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
    mocks.ReadFrom.mockResolvedValueOnce([{ ID: '1', Key: '1', Topic: 'source:test', Ts: 0, Payload: {} }])
    mocks.Commit.mockImplementationOnce(async () => { callOrder.push('commit') })
    mocks.FeedItemCounts.mockImplementationOnce(async () => { callOrder.push('refresh'); return [] })

    const logHandler = mocks.On.mock.calls.find(([event]) => event === 'log:appended')?.[1] as (() => void) | undefined
    expect(logHandler).toBeDefined()

    logHandler?.()
    // The drain yields between pages before its terminating empty read.
    await vi.waitFor(() => expect(callOrder).toEqual(['commit', 'refresh']))

    wrapper.unmount()
  })
})
