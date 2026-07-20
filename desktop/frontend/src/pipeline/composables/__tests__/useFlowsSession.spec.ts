import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { useFlowsSession, resetFlowsSessionForTests, type FlowsSessionDeps } from '../useFlowsSession'
import type { PipelineEditorClient } from '../usePipelineEditor'
import type { PipelineClient } from '../../driver'
import type { FlowSummary, WireFlow } from '../../lib/wireFlow'
import type { Msg } from '../../types'

function summary(id: string, overrides: Partial<FlowSummary> = {}): FlowSummary {
  return { id, name: id, enabled: true, valid: true, ...overrides }
}

// A single `feed` node is enough for the engine to run cleanly (no wiring
// needed) — same minimal fixture usePipelineRuntime.spec.ts uses for its
// Flow, just expressed in the wire shape GetFlow/SaveFlow speak.
function wireFlow(id = 'flow-1'): WireFlow {
  return { id, name: 'My flow', enabled: true, nodes: [{ id: 'feed', type: 'feed', feed: 'inbox' }], wires: [] }
}

function msg(id: string): Msg {
  return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: {}, Meta: null }
}

function fakeEditorClient(overrides: Partial<PipelineEditorClient> = {}): PipelineEditorClient {
  return {
    listFlows: vi.fn().mockResolvedValue([summary('flow-1')]),
    getFlow: vi.fn().mockResolvedValue(wireFlow()),
    saveFlow: vi.fn().mockResolvedValue(undefined),
    getLayout: vi.fn().mockResolvedValue({ nodes: {} }),
    saveLayout: vi.fn().mockResolvedValue(undefined),
    nodeRuns: vi.fn().mockResolvedValue([]),
    ...overrides,
  }
}

function fakeRuntimeClient(overrides: Partial<PipelineClient> = {}): PipelineClient {
  return {
    readFrom: vi.fn().mockResolvedValue([]),
    commit: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
}

// useFlowsSession() calls usePipelineEditor() internally, which registers
// onMounted/onUnmounted (its node_run poll timer) and this file's own
// runtime-rebuild watch() — both need an active component instance/effect
// scope, so (same pattern as usePipelineEditor.spec.ts's mountEditor)
// every test mounts a trivial host component rather than calling
// useFlowsSession() bare.
function mountSession(deps: FlowsSessionDeps = {}) {
  let state!: ReturnType<typeof useFlowsSession>
  const wrapper = mount({
    template: '<div />',
    setup() {
      state = useFlowsSession(deps)
      return {}
    },
  })
  return { state, wrapper }
}

// useFlowsSession is a module singleton — without a reset, a later test's
// mountSession() would silently reuse an earlier test's instance (complete
// with its already-torn-down hooks from that test's wrapper.unmount()).
beforeEach(() => {
  resetFlowsSessionForTests()
})

describe('useFlowsSession', () => {
  it('is a singleton: every call returns the same instance, and deps are only consulted on the first call', async () => {
    const firstEditorClient = fakeEditorClient()
    const { state: first, wrapper: firstWrapper } = mountSession({ editorClient: firstEditorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    const secondEditorClient = fakeEditorClient()
    const second = useFlowsSession({ editorClient: secondEditorClient, runtimeClient: fakeRuntimeClient() })

    expect(second).toBe(first)
    expect(secondEditorClient.listFlows).not.toHaveBeenCalled() // ignored — the first client already won
    firstWrapper.unmount()

    resetFlowsSessionForTests()
    const { state: third, wrapper: thirdWrapper } = mountSession({ editorClient: fakeEditorClient(), runtimeClient: fakeRuntimeClient() })
    await flushPromises()
    expect(third).not.toBe(first)
    thirdWrapper.unmount()
  })

  it('bindActiveFlow selects the flow once it is present in the loaded flows list', async () => {
    const editorClient = fakeEditorClient()
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises() // initial refreshFlows() from usePipelineEditor's onMounted

    expect(state.activeFlow.value).toBeNull()
    state.bindActiveFlow('flow-1')
    await flushPromises()

    expect(editorClient.getFlow).toHaveBeenCalledWith('flow-1')
    expect(state.activeFlow.value?.id).toBe('flow-1')

    wrapper.unmount()
  })

  it('bindActiveFlow is a no-op until the flows list actually contains the id, then re-checks once it arrives', async () => {
    let resolveList!: (v: FlowSummary[]) => void
    const listFlows = vi.fn().mockImplementation(() => new Promise<FlowSummary[]>((resolve) => { resolveList = resolve }))
    const editorClient = fakeEditorClient({ listFlows })
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })

    state.bindActiveFlow('flow-1') // flows hasn't resolved yet
    await flushPromises()
    expect(editorClient.getFlow).not.toHaveBeenCalled()

    resolveList([summary('flow-1')])
    await flushPromises()

    expect(editorClient.getFlow).toHaveBeenCalledWith('flow-1')
    expect(state.activeFlow.value?.id).toBe('flow-1')

    wrapper.unmount()
  })

  it('openFlows/exitFlows toggle flowsOpen and set the focus node for the 8d nav task', async () => {
    const { state, wrapper } = mountSession({ editorClient: fakeEditorClient(), runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    expect(state.flowsOpen.value).toBe(false)
    expect(state.flowFocusNodeId.value).toBeNull()

    state.openFlows('node-1')
    expect(state.flowsOpen.value).toBe(true)
    expect(state.flowFocusNodeId.value).toBe('node-1')

    state.exitFlows()
    expect(state.flowsOpen.value).toBe(false)

    // openFlows() with no argument clears any stale focus target.
    state.openFlows()
    expect(state.flowFocusNodeId.value).toBeNull()

    wrapper.unmount()
  })

  it('starts the runtime the moment a flow becomes active, and pump() drives a read+commit cycle', async () => {
    const editorClient = fakeEditorClient()
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn().mockResolvedValue(undefined)
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: { readFrom, commit } })
    await flushPromises()

    expect(state.running.value).toBe(false) // no flow bound yet

    state.bindActiveFlow('flow-1')
    await flushPromises()

    // run() fires automatically once the flow becomes active — an
    // immediate pump, exactly like FlowsView's old per-canvas runtime.
    expect(state.running.value).toBe(true)
    expect(readFrom).toHaveBeenCalledTimes(1)
    expect(state.lastRun.value).toMatchObject({ batchSize: 0 })

    readFrom.mockResolvedValueOnce([msg('1')])
    await state.pump()

    // The coalescing runtime drains the processed page and then performs its
    // terminating empty read before resolving.
    expect(readFrom).toHaveBeenCalledTimes(3)
    expect(commit).toHaveBeenCalledTimes(1)
    expect(state.lastRun.value).toMatchObject({ batchSize: 1, outputCount: 1 })
    expect(state.runtimeError.value).toBeNull()

    wrapper.unmount()
  })

  it('pump() is a clean no-op before any flow has been bound', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn()
    const { state, wrapper } = mountSession({ editorClient: fakeEditorClient(), runtimeClient: { readFrom, commit } })
    await flushPromises()

    await state.pump()

    expect(readFrom).not.toHaveBeenCalled()
    expect(commit).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('runRuntime/stopRuntime manually control the active flow\'s runtime (FlowsView\'s deploy-menu Run/Stop)', async () => {
    const editorClient = fakeEditorClient()
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn().mockResolvedValue(undefined)
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: { readFrom, commit } })
    await flushPromises()
    state.bindActiveFlow('flow-1')
    await flushPromises()
    expect(state.running.value).toBe(true)

    state.stopRuntime()
    expect(state.running.value).toBe(false)

    await state.runRuntime()
    expect(state.running.value).toBe(true)

    wrapper.unmount()
  })

  it('discardDraft() reloads the active flow fresh from disk, clearing dirty (hc-sx4k3c7k)', async () => {
    const editorClient = fakeEditorClient()
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    state.bindActiveFlow('flow-1')
    await flushPromises()
    expect(state.activeFlow.value?.id).toBe('flow-1')

    state.addNode('feed') // a local edit — dirties the draft
    expect(state.dirty.value).toBe(true)

    const getFlowCallsBefore = (editorClient.getFlow as ReturnType<typeof vi.fn>).mock.calls.length
    await state.discardDraft()

    expect(state.dirty.value).toBe(false)
    // Reloaded from disk via the same getFlow/getLayout path selectFlow uses.
    expect((editorClient.getFlow as ReturnType<typeof vi.fn>).mock.calls.length).toBe(getFlowCallsBefore + 1)
    expect(editorClient.getFlow).toHaveBeenLastCalledWith('flow-1')

    wrapper.unmount()
  })

  it('discardDraft() is a no-op when no flow is bound', async () => {
    const editorClient = fakeEditorClient()
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    expect(state.activeFlow.value).toBeNull()
    await state.discardDraft()

    expect(editorClient.getFlow).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('keeps undeployed draft edits out of the running snapshot', async () => {
    const editorClient = fakeEditorClient()
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn().mockResolvedValue(undefined)
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: { readFrom, commit } })
    await flushPromises()

    state.bindActiveFlow('flow-1')
    await flushPromises()
    state.addNode('feed') // a second terminal in the private editor draft
    expect(state.dirty.value).toBe(true)

    readFrom.mockReset()
    readFrom.mockResolvedValueOnce([msg('1')]).mockResolvedValueOnce([])
    await state.pump()

    // The deployed snapshot still has its single original feed node. If the
    // runtime had retained the mutable draft this page would emit two outputs.
    expect(commit).toHaveBeenLastCalledWith(expect.objectContaining({
      consumer: 'flow-1',
      outputs: [expect.any(Object)],
    }))
    wrapper.unmount()
  })

  it('only profile binding selects the runtime; choosing another editor draft cannot diverge it', async () => {
    const editorClient = fakeEditorClient({
      listFlows: vi.fn().mockResolvedValue([summary('flow-1'), summary('flow-2')]),
      getFlow: vi.fn().mockImplementation((id: string) => Promise.resolve(wireFlow(id))),
    })
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    state.bindActiveFlow('flow-1')
    await flushPromises()
    expect(state.runtimeFlowId.value).toBe('flow-1')

    await state.selectFlow('flow-2')
    expect(state.activeFlow.value?.id).toBe('flow-2')
    expect(state.runtimeFlowId.value).toBe('flow-1')

    state.bindActiveFlow('flow-2')
    // An old runtime is never reported as the new profile while its final
    // drain/swap is pending.
    expect(state.runtimeFlowId.value).not.toBe('flow-1')
    await flushPromises()
    expect(state.runtimeFlowId.value).toBe('flow-2')

    wrapper.unmount()
  })

  it('drains a queued pump before swapping to another profile runtime', async () => {
    const order: string[] = []
    const getFlow = vi.fn().mockImplementation(async (id: string) => {
      if (id === 'flow-2') order.push('swap')
      return wireFlow(id)
    })
    const editorClient = fakeEditorClient({
      listFlows: vi.fn().mockResolvedValue([summary('flow-1'), summary('flow-2')]),
      getFlow,
    })
    let resolveRead!: (batch: Msg[]) => void
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn().mockImplementation(async () => { order.push('commit') })
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: { readFrom, commit } })
    await flushPromises()

    state.bindActiveFlow('flow-1')
    await flushPromises()
    readFrom.mockImplementationOnce(() => new Promise<Msg[]>((resolve) => { resolveRead = resolve }))

    const drain = state.pump()
    await vi.waitFor(() => expect(readFrom).toHaveBeenCalledTimes(2))
    state.bindActiveFlow('flow-2')
    await flushPromises()
    expect(order).not.toContain('swap')

    resolveRead([msg('1')])
    await drain
    await flushPromises()

    expect(order).toEqual(expect.arrayContaining(['commit', 'swap']))
    expect(order.indexOf('commit')).toBeLessThan(order.indexOf('swap'))
    expect(state.runtimeFlowId.value).toBe('flow-2')
    wrapper.unmount()
  })

  it('keeps the last-good runtime when an external deployed reload fails', async () => {
    const editorClient = fakeEditorClient()
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    state.bindActiveFlow('flow-1')
    await flushPromises()
    ;(editorClient.getFlow as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('disk unavailable'))

    await state.reloadDeployed()

    expect(state.runtimeFlowId.value).toBe('flow-1')
    expect(state.running.value).toBe(true)
    expect(state.runtimeError.value).toBe('disk unavailable')
    wrapper.unmount()
  })

  it('rebuilds the runtime when the active flow changes to a different flow', async () => {
    const editorClient = fakeEditorClient({
      listFlows: vi.fn().mockResolvedValue([summary('flow-1'), summary('flow-2')]),
      getFlow: vi.fn().mockImplementation((id: string) => Promise.resolve(wireFlow(id))),
    })
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn().mockResolvedValue(undefined)
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: { readFrom, commit } })
    await flushPromises()

    state.bindActiveFlow('flow-1')
    await flushPromises()
    expect(readFrom).toHaveBeenCalledTimes(1) // initial pump for flow-1

    state.bindActiveFlow('flow-2')
    await flushPromises()

    expect(state.activeFlow.value?.id).toBe('flow-2')
    expect(state.running.value).toBe(true) // the new runtime auto-ran too
    expect(readFrom).toHaveBeenCalledTimes(2) // initial pump for flow-2

    wrapper.unmount()
  })
})
