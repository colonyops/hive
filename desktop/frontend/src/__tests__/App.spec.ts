import { describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import App from '../App.vue'
import { useCommandPalette } from '../composables/useCommands'
import { resetFlowsSessionForTests } from '../pipeline/composables/useFlowsSession'

const mocks = vi.hoisted(() => ({
  // flowsservice
  ListFlows: vi.fn(),
  GetFlow: vi.fn(),
  CreateFlow: vi.fn(),
  DeleteFlow: vi.fn(),
  GetLayout: vi.fn(),
  SaveFlow: vi.fn(),
  SaveLayout: vi.fn(),
  // pipelineservice
  FeedItems: vi.fn(),
  FeedItemCounts: vi.fn(),
  MarkFeedItemRead: vi.fn(),
  ActionsFor: vi.fn(),
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
}))

vi.mock('../../bindings/github.com/colonyops/hive/desktop/pipelineservice', () => ({
  FeedItems: mocks.FeedItems,
  FeedItemCounts: mocks.FeedItemCounts,
  MarkFeedItemRead: mocks.MarkFeedItemRead,
  ActionsFor: mocks.ActionsFor,
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

async function mountApp() {
  const wrapper = mount(App)
  await flushPromises()
  return wrapper
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
    mocks.FeedItems.mockResolvedValue([])
    mocks.FeedItemCounts.mockResolvedValue([{ feedId: 'personal/desktop', total: 1, unread: 0 }])
    mocks.ActionsFor.mockResolvedValue([])
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

    await wrapper.find('[data-testid="sidebar-open-flows"]').trigger('click')
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

  it('deletes the active profile (flow) via the trash icon + confirm modal, then falls back to onboarding', async () => {
    const wrapper = await mountApp()

    await wrapper.find('[data-testid="sidebar-delete-profile"]').trigger('click')
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

  it('binds the active profile\'s flow to the shared session/runtime even with the flows canvas closed (hc-8ft4yhm6)', async () => {
    const wrapper = await mountApp()

    // GetLayout/NodeRuns are only ever called from usePipelineEditor's
    // selectFlow — never from useFeedState — so seeing them here proves
    // the always-on session bound and loaded the active profile's flow
    // even though the flows canvas was never opened.
    expect(wrapper.find('[data-testid="flows-view"]').exists()).toBe(false)
    expect(mocks.GetLayout).toHaveBeenCalledWith('personal')
    expect(mocks.NodeRuns).toHaveBeenCalledWith('personal', 100)

    wrapper.unmount()
  })

  it('on "log:appended", pumps the runtime (commit) BEFORE refreshing the feed — the commit must land before the re-read', async () => {
    const wrapper = await mountApp()

    const callOrder: string[] = []
    mocks.ReadFrom.mockResolvedValueOnce([{ ID: 'm1', Key: 'm1', Topic: 'source:test', Ts: 0, Payload: {}, Meta: null }])
    mocks.Commit.mockImplementationOnce(async () => { callOrder.push('commit') })
    mocks.FeedItemCounts.mockImplementationOnce(async () => { callOrder.push('refresh'); return [] })

    const logHandler = mocks.On.mock.calls.find(([event]) => event === 'log:appended')?.[1] as (() => void) | undefined
    expect(logHandler).toBeDefined()

    logHandler?.()
    await flushPromises()

    expect(callOrder).toEqual(['commit', 'refresh'])

    wrapper.unmount()
  })
})
