import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NodeInspector from '../NodeInspector.vue'
import { byType } from '../../registry'
import type { NodeRunRecord } from '../../lib/wireFlow'
import type { FlowNode } from '../../types'

const def = byType['github-filter']!

function node(overrides: Partial<FlowNode> = {}): FlowNode {
  return { id: 'filter', type: 'github-filter', config: {}, ...overrides }
}

function run(overrides: Partial<NodeRunRecord> = {}): NodeRunRecord {
  return { flowId: 'flow-1', nodeId: 'filter', ok: true, inCount: 24, outCount: 19, dropCount: 5, err: '', durMs: 5, endedAt: Date.now() * 1e6, ...overrides }
}

function mountInspector(props: { node?: FlowNode; run?: NodeRunRecord; running?: boolean; recentRuns?: NodeRunRecord[] } = {}) {
  return mount(NodeInspector, {
    props: {
      node: props.node ?? node(),
      def,
      run: props.run,
      running: props.running ?? false,
      recentRuns: props.recentRuns ?? [],
    },
  })
}

describe('NodeInspector', () => {
  it('renders the node type\'s label, and the node\'s own name when set', () => {
    const withoutName = mountInspector()
    expect(withoutName.get('[data-testid="node-inspector-title"]').text()).toBe('GitHub filter')
    withoutName.unmount()

    const withName = mountInspector({ node: node({ name: 'is:open' }) })
    expect(withName.get('[data-testid="node-inspector-title"]').text()).toBe('GitHub filter · is:open')
    withName.unmount()
  })

  it('shows "idle" when there is no run and the node isn\'t running', () => {
    const wrapper = mountInspector()
    expect(wrapper.get('[data-testid="node-inspector-status"]').text()).toBe('idle')
    wrapper.unmount()
  })

  it('shows "running…" when running is true, even with a prior run', () => {
    const wrapper = mountInspector({ run: run({ ok: true }), running: true })
    expect(wrapper.get('[data-testid="node-inspector-status"]').text()).toBe('running…')
    wrapper.unmount()
  })

  it('shows counts + age for a successful run', () => {
    const wrapper = mountInspector({ run: run({ ok: true, inCount: 24, outCount: 19 }) })
    expect(wrapper.get('[data-testid="node-inspector-status"]').text()).toContain('24 → 19')
    wrapper.unmount()
  })

  it('shows the error message for a failed run', () => {
    const wrapper = mountInspector({ run: run({ ok: false, err: '401 unauthorized' }) })
    expect(wrapper.get('[data-testid="node-inspector-status"]').text()).toBe('error: 401 unauthorized')
    wrapper.unmount()
  })

  it('shows "No runs yet" when recentRuns is empty', () => {
    const wrapper = mountInspector()
    expect(wrapper.find('[data-testid="node-inspector-recent-empty"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="node-inspector-recent-row"]')).toHaveLength(0)
    wrapper.unmount()
  })

  it('renders one RECENT row per run, newest-first as given, with an ok/error marker and age', () => {
    const wrapper = mountInspector({
      recentRuns: [
        run({ ok: true, inCount: 19, outCount: 19 }),
        run({ ok: false, err: 'boom' }),
      ],
    })

    const rows = wrapper.findAll('[data-testid="node-inspector-recent-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('✓')
    expect(rows[0]!.text()).toContain('19 → 19')
    expect(rows[1]!.text()).toContain('✕')
    expect(rows[1]!.text()).toContain('boom')

    wrapper.unmount()
  })

  it('clicking Edit emits edit', async () => {
    const wrapper = mountInspector()

    await wrapper.get('[data-testid="node-inspector-edit"]').trigger('click')

    expect(wrapper.emitted('edit')).toHaveLength(1)

    wrapper.unmount()
  })
})
