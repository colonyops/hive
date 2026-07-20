import { describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FlowsView from '../FlowsView.vue'
import { resetFlowsSessionForTests, useFlowsSession } from '../../composables/useFlowsSession'
import type { WireFlow } from '../../lib/wireFlow'

// FlowsView reads everything from the useFlowsSession() singleton (a real
// adapter over the generated Wails bindings by default), so — same posture
// as App.spec.ts, which exercises this same toolbar through the full app
// tree — the bindings modules are mocked here rather than injecting a fake
// PipelineEditorClient directly.
const mocks = vi.hoisted(() => ({
  ListFlows: vi.fn(),
  GetFlow: vi.fn(),
  GetLayout: vi.fn(),
  SaveFlow: vi.fn(),
  SaveLayout: vi.fn(),
  FeedItems: vi.fn(),
  NodeRuns: vi.fn(),
  ReadFrom: vi.fn(),
  Commit: vi.fn(),
  On: vi.fn(),
  SetText: vi.fn(),
}))

vi.mock('../../../../bindings/github.com/colonyops/hive/desktop/flowsservice', () => ({
  ListFlows: mocks.ListFlows,
  GetFlow: mocks.GetFlow,
  GetLayout: mocks.GetLayout,
  SaveFlow: mocks.SaveFlow,
  SaveLayout: mocks.SaveLayout,
}))

vi.mock('../../../../bindings/github.com/colonyops/hive/desktop/pipelineservice', () => ({
  FeedItems: mocks.FeedItems,
  NodeRuns: mocks.NodeRuns,
  ReadFrom: mocks.ReadFrom,
  Commit: mocks.Commit,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Clipboard: { SetText: mocks.SetText },
}))

const flowSummaries = [
  { id: 'flow-1', name: 'Flow one', enabled: true, valid: true },
  { id: 'flow-2', name: 'Flow two', enabled: true, valid: true },
]

function wireFlow(id: string, name: string): WireFlow {
  return { id, name, enabled: true, nodes: [{ id: 'feed', type: 'feed', feed: 'inbox' }], wires: [] }
}

async function mountFlowsView() {
  const wrapper = mount(FlowsView)
  await flushPromises()
  return wrapper
}

describe('FlowsView flow selector', () => {
  beforeEach(() => {
    // useFlowsSession is a module singleton — without a reset, a later
    // test's mount would silently reuse a prior test's already-torn-down
    // instance (see useFlowsSession.ts's module docs).
    resetFlowsSessionForTests()
    vi.clearAllMocks()
    mocks.ListFlows.mockResolvedValue(flowSummaries)
    mocks.GetFlow.mockImplementation(async (id: string) => wireFlow(id, id === 'flow-2' ? 'Flow two' : 'Flow one'))
    mocks.GetLayout.mockResolvedValue({ nodes: {} })
    mocks.NodeRuns.mockResolvedValue([])
    mocks.ReadFrom.mockResolvedValue([])
    mocks.Commit.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
  })

  it('does not expose the in-canvas new-flow input or Add button', async () => {
    const wrapper = await mountFlowsView()

    await wrapper.get('[data-testid="flow-selector-toggle"]').trigger('click')

    expect(wrapper.find('[data-testid="flow-selector-menu"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="flow-selector-new-name"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="flow-selector-new-submit"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('still lists existing flows, and clicking one switches the active flow (mirrors the profile rail)', async () => {
    const wrapper = await mountFlowsView()
    useFlowsSession().bindActiveFlow('flow-1')
    await flushPromises()

    expect(wrapper.get('[data-testid="flow-selector-toggle"]').text()).toContain('Flow one')

    await wrapper.get('[data-testid="flow-selector-toggle"]').trigger('click')
    await wrapper.get('[data-testid="flow-selector-option-flow-2"]').trigger('click')
    await flushPromises()

    expect(mocks.GetFlow).toHaveBeenCalledWith('flow-2')
    expect(wrapper.get('[data-testid="flow-selector-toggle"]').text()).toContain('Flow two')
    // Picking a flow also closes the menu.
    expect(wrapper.find('[data-testid="flow-selector-menu"]').exists()).toBe(false)

    wrapper.unmount()
  })
})

describe('FlowsView deploy menu', () => {
  beforeEach(() => {
    // useFlowsSession is a module singleton — without a reset, a later
    // test's mount would silently reuse a prior test's already-torn-down
    // instance (see useFlowsSession.ts's module docs).
    resetFlowsSessionForTests()
    vi.clearAllMocks()
    mocks.ListFlows.mockResolvedValue(flowSummaries)
    mocks.GetFlow.mockImplementation(async (id: string) => wireFlow(id, id === 'flow-2' ? 'Flow two' : 'Flow one'))
    mocks.GetLayout.mockResolvedValue({ nodes: {} })
    mocks.NodeRuns.mockResolvedValue([])
    mocks.ReadFrom.mockResolvedValue([])
    mocks.Commit.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
  })

  async function mountWithActiveFlow() {
    const wrapper = await mountFlowsView()
    useFlowsSession().bindActiveFlow('flow-1')
    await flushPromises()
    return wrapper
  }

  it('"Refresh now" triggers an immediate manual pump via the session and closes the menu', async () => {
    const wrapper = await mountWithActiveFlow()
    mocks.ReadFrom.mockClear()

    await wrapper.get('[data-testid="deploy-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="deploy-menu-refresh"]').trigger('click')
    await flushPromises()

    expect(mocks.ReadFrom).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="deploy-menu"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('"Refresh now" is disabled when no flow is active', async () => {
    const wrapper = await mountFlowsView()

    await wrapper.get('[data-testid="deploy-menu-toggle"]').trigger('click')

    expect(wrapper.get('[data-testid="deploy-menu-refresh"]').attributes('disabled')).toBeDefined()

    wrapper.unmount()
  })

  it('"Copy prompt" still copies the flow prompt', async () => {
    mocks.SetText.mockResolvedValue(undefined)
    const wrapper = await mountWithActiveFlow()

    await wrapper.get('[data-testid="deploy-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="deploy-menu-copy-prompt"]').trigger('click')
    await flushPromises()

    expect(mocks.SetText).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="copy-prompt-status"]').text()).toBe('Prompt copied')

    wrapper.unmount()
  })

  it('debug toggle is labeled "Show debug panel" / "Hide debug panel" and toggles FlowDebugPanel', async () => {
    const wrapper = await mountWithActiveFlow()

    await wrapper.get('[data-testid="deploy-menu-toggle"]').trigger('click')
    expect(wrapper.get('[data-testid="deploy-menu-debug-toggle"]').text()).toBe('Show debug panel')
    expect(wrapper.find('[data-testid="flow-debug-aside"]').exists()).toBe(false)

    await wrapper.get('[data-testid="deploy-menu-debug-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="flow-debug-aside"]').exists()).toBe(true)
    // Toggling also closes the menu — reopen it to check the label flipped.
    await wrapper.get('[data-testid="deploy-menu-toggle"]').trigger('click')
    expect(wrapper.get('[data-testid="deploy-menu-debug-toggle"]').text()).toBe('Hide debug panel')

    wrapper.unmount()
  })
})
