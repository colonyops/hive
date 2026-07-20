import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FlowDebugPanel from '../FlowDebugPanel.vue'
import type { EditorFlow, NodeRunRecord } from '../../lib/wireFlow'
import type { RuntimeSummary } from '../../composables/usePipelineRuntime'

function flow(): EditorFlow {
  return {
    id: 'flow-1',
    name: 'Flow one',
    enabled: true,
    nodes: [
      { id: 'src', type: 'github-source', config: { source: 'my-prs' } },
      { id: 'filter', type: 'github-filter', config: {} },
      { id: 'feed', type: 'feed', config: { feed: 'inbox' } },
    ],
    wires: [],
  }
}

function run(overrides: Partial<NodeRunRecord> = {}): NodeRunRecord {
  return { flowId: 'flow-1', nodeId: 'src', ok: true, inCount: 1, outCount: 1, dropCount: 0, err: '', durMs: 5, endedAt: 1_000_000_000, ...overrides }
}

describe('FlowDebugPanel', () => {
  it('shows idle for nodes with no recorded run', () => {
    const wrapper = mount(FlowDebugPanel, {
      props: { flow: flow(), latestRunByNode: new Map(), nodeRuns: [], runtimeSummary: null, running: false },
    })

    const rows = wrapper.findAll('[data-testid^="debug-node-"]')
    expect(rows).toHaveLength(3)
    for (const row of rows) expect(row.text()).toContain('idle')

    wrapper.unmount()
  })

  it("renders each node's latest status (ok/err, in→out→drop, durMs)", () => {
    const latestRunByNode = new Map<string, NodeRunRecord>([
      ['src', run({ nodeId: 'src', ok: true, inCount: 3, outCount: 3, dropCount: 0, durMs: 4 })],
      ['filter', run({ nodeId: 'filter', ok: false, err: 'boom', inCount: 2, outCount: 1, dropCount: 1, durMs: 9 })],
    ])
    const wrapper = mount(FlowDebugPanel, {
      props: { flow: flow(), latestRunByNode, nodeRuns: [], runtimeSummary: null, running: false },
    })

    const srcRow = wrapper.get('[data-testid="debug-node-src"]')
    expect(srcRow.text()).toContain('ok')
    expect(srcRow.text()).toContain('3→3→0')
    expect(srcRow.text()).toContain('4ms')

    const filterRow = wrapper.get('[data-testid="debug-node-filter"]')
    expect(filterRow.text()).toContain('err')
    expect(filterRow.text()).toContain('2→1→1')

    const feedRow = wrapper.get('[data-testid="debug-node-feed"]')
    expect(feedRow.text()).toContain('idle')

    wrapper.unmount()
  })

  it('renders a RECENT list of the last N node_run rows in the order given (newest first)', () => {
    const nodeRuns = [
      run({ nodeId: 'feed', endedAt: 300 }),
      run({ nodeId: 'filter', endedAt: 200, ok: false, err: 'nope' }),
      run({ nodeId: 'src', endedAt: 100 }),
    ]
    const wrapper = mount(FlowDebugPanel, {
      props: { flow: flow(), latestRunByNode: new Map(), nodeRuns, runtimeSummary: null, running: false },
    })

    const rows = wrapper.findAll('[data-testid="debug-recent-row"]')
    expect(rows).toHaveLength(3)
    expect(rows[0]!.text()).toContain('Feed')
    expect(rows[1]!.text()).toContain('GitHub filter')
    expect(rows[1]!.text()).toContain('nope')
    expect(rows[2]!.text()).toContain('GitHub source')

    wrapper.unmount()
  })

  it('shows an empty RECENT state when there is no activity yet', () => {
    const wrapper = mount(FlowDebugPanel, {
      props: { flow: flow(), latestRunByNode: new Map(), nodeRuns: [], runtimeSummary: null, running: false },
    })

    expect(wrapper.find('[data-testid="debug-recent-empty"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('sums durMs across only the nodes that share the latest tick\'s endedAt for the end-to-end figure', () => {
    const latestRunByNode = new Map<string, NodeRunRecord>([
      ['src', run({ nodeId: 'src', endedAt: 500, durMs: 10 })],
      ['filter', run({ nodeId: 'filter', endedAt: 500, durMs: 15 })],
      // An older run for a node that produced nothing this latest tick —
      // excluded from the end-to-end sum.
      ['feed', run({ nodeId: 'feed', endedAt: 100, durMs: 999 })],
    ])
    const wrapper = mount(FlowDebugPanel, {
      props: { flow: flow(), latestRunByNode, nodeRuns: [], runtimeSummary: null, running: false },
    })

    const el = wrapper.get('[data-testid="debug-end-to-end"]')
    expect(el.text()).toContain('25ms')
    expect(el.text()).toContain('2 nodes')

    wrapper.unmount()
  })

  it('renders the runtime summary line from usePipelineRuntime', () => {
    const summary: RuntimeSummary = { batchSize: 5, outputCount: 3, discardCount: 2, errorCount: 1, completedAt: Date.now() }
    const wrapper = mount(FlowDebugPanel, {
      props: { flow: flow(), latestRunByNode: new Map(), nodeRuns: [], runtimeSummary: summary, running: true },
    })

    const el = wrapper.get('[data-testid="debug-last-pump"]')
    expect(el.text()).toContain('5 msgs')
    expect(el.text()).toContain('3 outputs')
    expect(el.text()).toContain('2 discards')
    expect(el.text()).toContain('1 error')
    expect(wrapper.text()).toContain('Running')

    wrapper.unmount()
  })

  it('shows a no-pump-yet state and a Stopped indicator when idle', () => {
    const wrapper = mount(FlowDebugPanel, {
      props: { flow: flow(), latestRunByNode: new Map(), nodeRuns: [], runtimeSummary: null, running: false },
    })

    expect(wrapper.find('[data-testid="debug-last-pump-empty"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Stopped')

    wrapper.unmount()
  })
})
