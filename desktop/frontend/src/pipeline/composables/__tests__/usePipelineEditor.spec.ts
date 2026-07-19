import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { usePipelineEditor, type PipelineEditorClient } from '../usePipelineEditor'
import type { FlowSummary, NodeRunRecord, WireFlow, WireLayout } from '../../lib/wireFlow'

function wireFlow(overrides: Partial<WireFlow> = {}): WireFlow {
  return {
    id: 'flow-1',
    name: 'My flow',
    enabled: true,
    nodes: [
      { id: 'src', type: 'github-source', source: 'my-prs' },
      { id: 'feed', type: 'feed', feed: 'inbox' },
    ],
    wires: [{ from: 'src', to: 'feed' }],
    ...overrides,
  }
}

function summary(id: string, overrides: Partial<FlowSummary> = {}): FlowSummary {
  return { id, name: id, enabled: true, valid: true, ...overrides }
}

function fakeClient(overrides: Partial<PipelineEditorClient> = {}): PipelineEditorClient {
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

// usePipelineEditor uses onMounted/onUnmounted (the poll timer) — mount it
// inside a host component, same pattern as useFeedState.spec.ts, so those
// hooks actually fire and wrapper.unmount() cleans up the interval.
function mountEditor(client: PipelineEditorClient, options: Parameters<typeof usePipelineEditor>[1] = {}) {
  let state!: ReturnType<typeof usePipelineEditor>
  const wrapper = mount({
    template: '<div />',
    setup() {
      state = usePipelineEditor(client, options)
      return {}
    },
  })
  return { state, wrapper }
}

async function mountLoadedEditor(client: PipelineEditorClient = fakeClient()) {
  const mounted = mountEditor(client)
  await flushPromises()
  return mounted
}

describe('usePipelineEditor', () => {
  it('loads the flows list on mount', async () => {
    const client = fakeClient({ listFlows: vi.fn().mockResolvedValue([summary('a'), summary('b')]) })
    const { state, wrapper } = await mountLoadedEditor(client)

    expect(state.flows.value.map((f) => f.id)).toEqual(['a', 'b'])
    expect(state.loadingFlows.value).toBe(false)

    wrapper.unmount()
  })

  it('selectFlow loads the flow and layout, converts the wire shape, and clears dirty', async () => {
    const client = fakeClient({
      getFlow: vi.fn().mockResolvedValue(wireFlow()),
      getLayout: vi.fn().mockResolvedValue({ nodes: { src: { x: 10, y: 20 } } }),
    })
    const { state, wrapper } = await mountLoadedEditor(client)

    await state.selectFlow('flow-1')

    expect(client.getFlow).toHaveBeenCalledWith('flow-1')
    expect(client.getLayout).toHaveBeenCalledWith('flow-1')
    expect(state.activeFlow.value).toEqual({
      id: 'flow-1',
      name: 'My flow',
      enabled: true,
      nodes: [
        { id: 'src', type: 'github-source', config: { source: 'my-prs' } },
        { id: 'feed', type: 'feed', config: { feed: 'inbox' } },
      ],
      wires: [{ from: 'src', to: 'feed' }],
    })
    expect(state.layout.value).toEqual({ nodes: { src: { x: 10, y: 20 } } })
    expect(state.dirty.value).toBe(false)
    expect(client.nodeRuns).toHaveBeenCalledWith('flow-1', 100)

    wrapper.unmount()
  })

  it('selectFlow surfaces a load failure and clears the active flow', async () => {
    const client = fakeClient({ getFlow: vi.fn().mockRejectedValue(new Error('boom')) })
    const { state, wrapper } = await mountLoadedEditor(client)

    await state.selectFlow('flow-1')

    expect(state.activeFlow.value).toBeNull()
    expect(state.error.value).toBe('boom')

    wrapper.unmount()
  })

  it('addNode instantiates from the registry, appends to the draft, places a layout position, and marks dirty', async () => {
    const client = fakeClient()
    const { state, wrapper } = await mountLoadedEditor(client)
    await state.selectFlow('flow-1')

    const before = state.activeFlow.value!.nodes.length
    const node = state.addNode('feed')

    expect(node.type).toBe('feed')
    expect(state.activeFlow.value!.nodes).toHaveLength(before + 1)
    expect(state.activeFlow.value!.nodes.at(-1)).toEqual(node)
    // The fixture flow already has 2 nodes (src, feed) — the new node lands
    // at grid index 2 (third slot: col 2, row 0).
    expect(state.layout.value.nodes?.[node.id]).toEqual({ x: 600, y: 80 })
    expect(state.dirty.value).toBe(true)

    wrapper.unmount()
  })

  it('updateNode replaces the node by id and marks dirty', async () => {
    const { state, wrapper } = await mountLoadedEditor()
    await state.selectFlow('flow-1')
    state.dirty.value = false

    state.updateNode({ id: 'feed', type: 'feed', name: 'Renamed', config: { feed: 'inbox' } })

    expect(state.activeFlow.value!.nodes.find((n) => n.id === 'feed')).toEqual({
      id: 'feed', type: 'feed', name: 'Renamed', config: { feed: 'inbox' },
    })
    expect(state.dirty.value).toBe(true)

    wrapper.unmount()
  })

  it('deleteNode removes the node, any of its wires, and its layout entry', async () => {
    const client = fakeClient({ getLayout: vi.fn().mockResolvedValue({ nodes: { src: { x: 1, y: 1 }, feed: { x: 2, y: 2 } } }) })
    const { state, wrapper } = await mountLoadedEditor(client)
    await state.selectFlow('flow-1')

    state.deleteNode('feed')

    expect(state.activeFlow.value!.nodes.map((n) => n.id)).toEqual(['src'])
    expect(state.activeFlow.value!.wires).toEqual([])
    expect(state.layout.value.nodes).toEqual({ src: { x: 1, y: 1 } })
    expect(state.dirty.value).toBe(true)

    wrapper.unmount()
  })

  it('addWire dedupes an identical wire and removeWire drops a matching one', async () => {
    const { state, wrapper } = await mountLoadedEditor()
    await state.selectFlow('flow-1')

    state.addWire({ from: 'src', to: 'feed' }) // already present — no-op
    expect(state.activeFlow.value!.wires).toHaveLength(1)

    state.addWire({ from: 'feed', out: 1, to: 'src' })
    expect(state.activeFlow.value!.wires).toHaveLength(2)

    state.removeWire({ from: 'src', to: 'feed' })
    expect(state.activeFlow.value!.wires).toEqual([{ from: 'feed', out: 1, to: 'src' }])

    wrapper.unmount()
  })

  it('moveNode updates the layout position and marks dirty', async () => {
    const { state, wrapper } = await mountLoadedEditor()
    await state.selectFlow('flow-1')
    state.dirty.value = false

    state.moveNode('src', 111, 222)

    expect(state.layout.value.nodes?.src).toEqual({ x: 111, y: 222 })
    expect(state.dirty.value).toBe(true)

    wrapper.unmount()
  })

  it('deploy saves the flow and layout in the wire shape, clears dirty, and refreshes the flows list', async () => {
    const client = fakeClient()
    const { state, wrapper } = await mountLoadedEditor(client)
    await state.selectFlow('flow-1')
    state.moveNode('src', 5, 6)
    ;(client.listFlows as ReturnType<typeof vi.fn>).mockClear()

    await state.deploy()

    expect(client.saveFlow).toHaveBeenCalledWith(wireFlow())
    expect(client.saveLayout).toHaveBeenCalledWith('flow-1', { nodes: { src: { x: 5, y: 6 } } })
    expect(state.dirty.value).toBe(false)
    expect(client.listFlows).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  it('deploy surfaces a save failure and leaves dirty set', async () => {
    const client = fakeClient({ saveFlow: vi.fn().mockRejectedValue(new Error('rejected')) })
    const { state, wrapper } = await mountLoadedEditor(client)
    await state.selectFlow('flow-1')
    state.moveNode('src', 1, 1)

    await state.deploy()

    expect(state.error.value).toBe('rejected')
    expect(state.dirty.value).toBe(true)

    wrapper.unmount()
  })

  it('newFlow starts an empty local draft, marks dirty, and adds an optimistic row to the flows list', async () => {
    const { state, wrapper } = await mountLoadedEditor()

    state.newFlow('My New Flow!')

    expect(state.activeFlow.value).toEqual({ id: 'my-new-flow', name: 'My New Flow!', enabled: true, nodes: [], wires: [] })
    expect(state.layout.value).toEqual({ nodes: {} })
    expect(state.dirty.value).toBe(true)
    expect(state.flows.value.some((f) => f.id === 'my-new-flow')).toBe(true)

    wrapper.unmount()
  })

  it('newFlow de-duplicates the slug against existing flow ids', async () => {
    const client = fakeClient({ listFlows: vi.fn().mockResolvedValue([summary('inbox'), summary('inbox-2')]) })
    const { state, wrapper } = await mountLoadedEditor(client)

    state.newFlow('Inbox')

    expect(state.activeFlow.value!.id).toBe('inbox-3')

    wrapper.unmount()
  })

  it('derives the latest node_run per node from a newest-first list, ignoring later (older) duplicates', async () => {
    const runs: NodeRunRecord[] = [
      { flowId: 'flow-1', nodeId: 'a', ok: true, inCount: 2, outCount: 2, dropCount: 0, err: '', durMs: 5, endedAt: 300 },
      { flowId: 'flow-1', nodeId: 'b', ok: false, inCount: 1, outCount: 0, dropCount: 1, err: 'boom', durMs: 1, endedAt: 200 },
      { flowId: 'flow-1', nodeId: 'a', ok: true, inCount: 1, outCount: 1, dropCount: 0, err: '', durMs: 3, endedAt: 100 },
    ]
    const client = fakeClient({ nodeRuns: vi.fn().mockResolvedValue(runs) })
    const { state, wrapper } = await mountLoadedEditor(client)

    await state.selectFlow('flow-1')

    expect(state.nodeRuns.value).toEqual(runs)
    expect(state.latestRunByNode.value.get('a')).toEqual(runs[0])
    expect(state.latestRunByNode.value.get('b')).toEqual(runs[1])
    expect(state.latestRunByNode.value.size).toBe(2)

    wrapper.unmount()
  })

  it('polls nodeRuns on an interval while mounted', async () => {
    vi.useFakeTimers()
    try {
      const client = fakeClient()
      const { state, wrapper } = mountEditor(client, { pollIntervalMs: 1000 })
      await vi.advanceTimersByTimeAsync(0) // flush the mounted-hook refreshFlows()
      await state.selectFlow('flow-1')
      ;(client.nodeRuns as ReturnType<typeof vi.fn>).mockClear()

      await vi.advanceTimersByTimeAsync(1000)
      expect(client.nodeRuns).toHaveBeenCalledTimes(1)

      await vi.advanceTimersByTimeAsync(2000)
      expect(client.nodeRuns).toHaveBeenCalledTimes(3)

      wrapper.unmount()
      await vi.advanceTimersByTimeAsync(5000)
      expect(client.nodeRuns).toHaveBeenCalledTimes(3) // stopped after unmount
    } finally {
      vi.useRealTimers()
    }
  })
})
