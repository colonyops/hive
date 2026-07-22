import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { useFlowsSession, resetFlowsSessionForTests, type FlowsSessionDeps } from '../useFlowsSession'
import type { PipelineEditorClient } from '../usePipelineEditor'
import type { PipelineClient } from '../../driver'
import { usePipelineRuntime } from '../usePipelineRuntime'
import type { WorkerTransport } from '../../engine/transport'
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

function replayWireFlow(id = 'flow-1'): WireFlow {
  return {
    id, name: 'Replay flow', enabled: true,
    nodes: [
      { id: 'source', type: 'github-source', kind: 'search', query: 'is:open' },
      { id: 'feed', type: 'feed', feed: 'inbox' },
      { id: 'action', type: 'action', action: 'would-run-if-not-snapshot' },
    ],
    wires: [{ from: 'source', to: 'feed' }, { from: 'source', to: 'action' }],
  }
}

function msg(id: string): Msg {
  return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: {} }
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

  // Regression: creating a profile switches selectedProfileId before its flow
  // reaches the editor's list, so the bind defers. Leaving the previous
  // profile's draft active meanwhile let the user rename a node against the
  // wrong flow; the deferred selectFlow/replaceDraft then discarded that edit
  // (dirty cleared → Deploy greyed out) — the webkit onboarding e2e flake at
  // onboarding.spec.ts:101. The stale draft must be dropped on the deferred bind.
  it('clears the previous draft when binding to a not-yet-loaded flow, then binds it once it arrives', async () => {
    let flowsList = [summary('flow-1')]
    const editorClient = fakeEditorClient({
      listFlows: vi.fn().mockImplementation(() => Promise.resolve(flowsList)),
      getFlow: vi.fn().mockImplementation((id: string) => Promise.resolve(wireFlow(id))),
    })
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    state.bindActiveFlow('flow-1')
    await flushPromises()
    expect(state.activeFlow.value?.id).toBe('flow-1')

    // Switch to a profile whose flow is not in the list yet (deferred bind).
    state.bindActiveFlow('flow-2')
    await flushPromises()
    // The stale flow-1 draft must be gone — not left editable under flow-2.
    expect(state.activeFlow.value).toBeNull()

    // flow-2 lands (its flows:updated); the deferred bind completes correctly.
    flowsList = [summary('flow-1'), summary('flow-2')]
    await state.reloadDeployed()
    await flushPromises()
    expect(state.activeFlow.value?.id).toBe('flow-2')
    expect(state.dirty.value).toBe(false)

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

  it('fast-forwards stale action-bound backlog, recomputes claims, then starts normal pumping', async () => {
    const order: string[] = []
    let fastForwarded = false
    let replayOutputs: any[] = []
    const staleActionBoundBacklog = [{ ...msg('7'), Topic: 'source:flow-1/source', SourceKind: 'github', SourceScope: 'acme/repo', OccurrenceKey: 'stale-action' }]
    const readFrom = vi.fn(async () => {
      order.push('read')
      // This models SQLite's persisted consumer checkpoint: before the
      // startup fast-forward this page would reach the action terminal.
      if (!fastForwarded) return staleActionBoundBacklog
      return []
    })
    const commit = vi.fn().mockResolvedValue(undefined)
    const runtimeClient = {
      readFrom,
      commit,
      eventLogTailOffset: vi.fn(async () => { order.push('tail'); return '7' }),
      fastForwardConsumer: vi.fn(async () => { order.push('fast-forward'); fastForwarded = true }),
      listUnarchivedInboxItems: vi.fn(async () => {
        order.push('items')
        return [{ id: 42, profileId: 'flow-1', sourceKind: 'github', sourceScope: 'acme/repo', externalId: 'item-1', title: 'Item', url: '', payload: { title: 'actual payload' }, revision: 1, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: 1 }]
      }),
      recomputeMemberships: vi.fn(async () => { order.push('memberships') }),
      reconcileFlowMembershipStructure: vi.fn(async () => { order.push('reconcile') }),
    }
    const runtimeFactory: typeof usePipelineRuntime = (client, flow) => {
      const runtime = usePipelineRuntime(client, flow, { transport: { run: vi.fn(), reset: vi.fn(), dispose: vi.fn() } })
      return {
        ...runtime,
        async recompute(messages) {
          order.push('recompute')
          const result = await runtime.recompute(messages)
          replayOutputs = result.outputs
          return result
        },
        async run() {
          order.push('run')
          await runtime.run()
        },
      }
    }
    const editorClient = fakeEditorClient({ getFlow: vi.fn().mockResolvedValue(replayWireFlow()) })
    const { wrapper } = mountSession({ editorClient, runtimeClient, runtimeFactory })
    await flushPromises()

    expect(order).toEqual(['tail', 'fast-forward', 'items', 'recompute', 'memberships', 'reconcile', 'run', 'read'])
    expect(readFrom).toHaveBeenCalledOnce()
    expect(commit).not.toHaveBeenCalled()
    expect(replayOutputs).toEqual([expect.objectContaining({ sink: { kind: 'feed', targetId: 'flow-1/feed' }, key: 'item-1', sourceKind: 'github', sourceScope: 'acme/repo' })])
    expect(replayOutputs.some((output) => output.sink.kind === 'action')).toBe(false)
    expect(runtimeClient.recomputeMemberships).toHaveBeenCalledWith('flow-1', [{ profile_id: 'flow-1', feed_id: 'flow-1/feed', item_id: 42, source_id: 'source:flow-1/source' }])
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

  it('starts every enabled flow before a profile is bound and pump() drains each runtime', async () => {
    const editorClient = fakeEditorClient({
      listFlows: vi.fn().mockResolvedValue([summary('flow-1'), summary('flow-2'), summary('disabled', { enabled: false })]),
      getFlow: vi.fn().mockImplementation((id: string) => Promise.resolve(wireFlow(id))),
    })
    let acceptWork = false
    let delivered = false
    const readFrom = vi.fn().mockImplementation(async (consumer: string) => {
      if (acceptWork && consumer === 'flow-2' && !delivered) {
        delivered = true
        return [msg('1')]
      }
      return []
    })
    const commit = vi.fn().mockResolvedValue(undefined)
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: { readFrom, commit } })
    await flushPromises()

    // No profile/canvas selection has happened, but both enabled flows have
    // their own durable consumer and started their initial backlog drain.
    expect(editorClient.getFlow).toHaveBeenCalledWith('flow-1')
    expect(editorClient.getFlow).toHaveBeenCalledWith('flow-2')
    expect(editorClient.getFlow).not.toHaveBeenCalledWith('disabled')
    expect(readFrom.mock.calls.map(([consumer]) => consumer)).toEqual(expect.arrayContaining(['flow-1', 'flow-2']))

    readFrom.mockClear()
    commit.mockClear()
    acceptWork = true
    await state.pump()

    expect(readFrom.mock.calls.map(([consumer]) => consumer)).toEqual(expect.arrayContaining(['flow-1', 'flow-2']))
    expect(commit).toHaveBeenCalledWith(expect.objectContaining({ consumer: 'flow-2' }))
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

  it('discardDraft() is a no-op when no editor flow is selected', async () => {
    const editorClient = fakeEditorClient()
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()

    expect(state.activeFlow.value).toBeNull()
    const getFlowCalls = (editorClient.getFlow as ReturnType<typeof vi.fn>).mock.calls.length
    await state.discardDraft()

    // Runtime startup loads enabled deployments, but discard itself does not
    // issue a second editor read without an editor selection.
    expect(editorClient.getFlow).toHaveBeenCalledTimes(getFlowCalls)

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

  it('disposes abandoned runtimes when replacing them and on session shutdown', async () => {
    const transports: WorkerTransport[] = []
    const runtimeFactory: typeof usePipelineRuntime = (client, flow) => {
      const transport: WorkerTransport = { run: vi.fn(), reset: vi.fn(), dispose: vi.fn() }
      transports.push(transport)
      return usePipelineRuntime(client, flow, { transport })
    }
    const { state, wrapper } = mountSession({
      editorClient: fakeEditorClient(),
      runtimeClient: fakeRuntimeClient(),
      runtimeFactory,
    })
    await flushPromises()
    expect(transports).toHaveLength(1)

    await state.reloadDeployed()
    expect(transports).toHaveLength(2)
    expect(transports[0].dispose).toHaveBeenCalledOnce()

    state.disposeRuntime()
    expect(transports[1].dispose).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('reconciles disabled and deleted flows by stopping their runtimes', async () => {
    const listFlows = vi.fn()
      .mockResolvedValueOnce([summary('flow-1'), summary('flow-2')])
      .mockResolvedValueOnce([summary('flow-1', { enabled: false }), summary('flow-2')])
      .mockResolvedValueOnce([summary('flow-2')]) // flow-1 was then deleted externally
    const editorClient = fakeEditorClient({
      listFlows,
      getFlow: vi.fn().mockImplementation((id: string) => Promise.resolve(wireFlow(id))),
    })
    const readFrom = vi.fn().mockResolvedValue([])
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: { readFrom, commit: vi.fn() } })
    await flushPromises()

    await state.reloadDeployed() // flow-1 becomes disabled
    readFrom.mockClear()
    await state.pump()
    expect(readFrom.mock.calls.map(([consumer]) => consumer)).toEqual(['flow-2'])

    await state.reloadDeployed() // flow-1 is now absent from the listing
    readFrom.mockClear()
    await state.pump()
    expect(readFrom.mock.calls.map(([consumer]) => consumer)).toEqual(['flow-2'])
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

  it('reloadDeployed rebuilds every enabled runtime, not just the selected profile', async () => {
    const getFlow = vi.fn().mockImplementation((id: string) => Promise.resolve(wireFlow(id)))
    const editorClient = fakeEditorClient({
      listFlows: vi.fn().mockResolvedValue([summary('flow-1'), summary('flow-2')]),
      getFlow,
    })
    const { state, wrapper } = mountSession({ editorClient, runtimeClient: fakeRuntimeClient() })
    await flushPromises()
    expect(getFlow).toHaveBeenCalledTimes(2)

    state.bindActiveFlow('flow-1')
    await flushPromises()
    const beforeReload = getFlow.mock.calls.length
    await state.reloadDeployed()

    expect(getFlow.mock.calls.slice(beforeReload).map(([id]) => id)).toEqual(expect.arrayContaining(['flow-1', 'flow-2']))
    wrapper.unmount()
  })
})
