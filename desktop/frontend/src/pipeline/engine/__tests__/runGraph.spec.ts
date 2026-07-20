import { describe, expect, it, vi } from 'vitest'
import { runGraph, UNROUTED_NODE_ID } from '../runGraph'
import { InProcessTransport, NodeTimeoutError, type WorkerTransport } from '../transport'
import { processorRegistry } from '../../registry'
import type { Flow, Msg } from '../../types'

function msg(id: string, payload: any = {}, topic = 'source:test'): Msg {
  return { ID: id, Key: id, Topic: topic, Ts: 0, Payload: payload, Meta: null }
}

describe('runGraph', () => {
  it('executes nodes in topological order, forwarding along wires', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const flow: Flow = {
      id: 'chain',
      nodes: [
        { id: 'a', type: 'function', config: { on_message: 'msg.Payload.trail = ["a"]; return msg' } },
        { id: 'b', type: 'function', config: { on_message: 'msg.Payload.trail.push("b"); return msg' } },
        { id: 'c', type: 'function', config: { on_message: 'msg.Payload.trail.push("c"); return msg' } },
        { id: 'out', type: 'feed', config: { feed: 'inbox' } },
      ],
      wires: [
        { from: 'a', to: 'b' },
        { from: 'b', to: 'c' },
        { from: 'c', to: 'out' },
      ],
    }
    const result = await runGraph(flow, [msg('1')], transport)
    expect(result.outputs).toHaveLength(1)
    expect(result.outputs[0].payload.trail).toEqual(['a', 'b', 'c'])
  })

  it('a disabled node discards its msg without invoking the transport', async () => {
    const runSpy = vi.fn()
    const transport: WorkerTransport = { run: runSpy, reset: vi.fn() }
    const flow: Flow = {
      id: 'f',
      nodes: [{ id: 'n', type: 'function', disabled: true, config: { on_message: 'return msg' } }],
      wires: [],
    }
    const result = await runGraph(flow, [msg('1')], transport)
    expect(result.discards).toEqual([{ msgId: '1', nodeId: 'n' }])
    expect(runSpy).not.toHaveBeenCalled()
    expect(result.nodeRuns.find((r) => r.nodeId === 'n')).toMatchObject({ inCount: 1, outCount: 0, dropCount: 1, ok: true })
  })

  it('an output emitted on an unwired port becomes a discard', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const flow: Flow = {
      id: 'f',
      nodes: [{ id: 'n', type: 'function', config: { on_message: 'return msg' } }],
      wires: [],
    }
    const result = await runGraph(flow, [msg('1')], transport)
    expect(result.discards).toEqual([{ msgId: '1', nodeId: 'n' }])
    expect(result.outputs).toEqual([])
    expect(result.nodeRuns.find((r) => r.nodeId === 'n')).toMatchObject({ inCount: 1, outCount: 0, dropCount: 1 })
  })

  it('a node returning null discards the msg', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const flow: Flow = {
      id: 'f',
      nodes: [
        { id: 'n', type: 'function', config: { on_message: 'return null' } },
        { id: 'out', type: 'feed', config: { feed: 'inbox' } },
      ],
      wires: [{ from: 'n', to: 'out' }],
    }
    const result = await runGraph(flow, [msg('1')], transport)
    expect(result.discards).toEqual([{ msgId: '1', nodeId: 'n' }])
    expect(result.outputs).toEqual([])
  })

  it('deep-clones on fan-out so mutating one branch never affects a sibling branch', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const flow: Flow = {
      id: 'f',
      nodes: [
        { id: 'src', type: 'function', config: { on_message: 'return msg' } },
        { id: 'mutator', type: 'function', config: { on_message: 'msg.Payload.x = "mutated"; return msg' } },
        { id: 'reader', type: 'function', config: { on_message: 'return msg' } },
        { id: 'mutated-feed', type: 'feed', config: { feed: 'mutated' } },
        { id: 'reader-feed', type: 'feed', config: { feed: 'reader' } },
      ],
      wires: [
        { from: 'src', to: 'mutator' },
        { from: 'src', to: 'reader' },
        { from: 'mutator', to: 'mutated-feed' },
        { from: 'reader', to: 'reader-feed' },
      ],
    }
    const result = await runGraph(flow, [msg('1', { x: 'original' })], transport)
    // A feed node's sink target is its flow-qualified node id (<flowId>/<nodeId>).
    const mutatedOut = result.outputs.find((o) => o.sink.targetId === 'f/mutated-feed')
    const readerOut = result.outputs.find((o) => o.sink.targetId === 'f/reader-feed')
    expect(mutatedOut?.payload.x).toBe('mutated')
    expect(readerOut?.payload.x).toBe('original')
  })

  it('produces the expected CommitBatch shape for a source -> filter -> feed/action flow', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const flow: Flow = {
      id: 'triage',
      nodes: [
        { id: 'in-prs', type: 'github-source', config: { kind: 'search', query: 'is:open' } },
        { id: 'drop-bots', type: 'github-filter', config: { repos: ['acme/*'] } },
        { id: 'team-feed', type: 'feed', config: {} },
        { id: 'spawn-review', type: 'action', config: { action: 'review-pr' } },
      ],
      wires: [
        { from: 'in-prs', to: 'drop-bots' },
        { from: 'drop-bots', out: 0, to: 'team-feed' },
        { from: 'drop-bots', out: 0, to: 'spawn-review' },
        // port 1 (fail) intentionally left unwired — today's plain drop behavior.
      ],
    }
    // The source node ingests only its own flow-qualified topic (source:<flowId>/<nodeId>).
    const passing = msg('1', { repo: 'acme/app' }, 'source:triage/in-prs')
    const failing = msg('2', { repo: 'other/repo' }, 'source:triage/in-prs')
    const result = await runGraph(flow, [passing, failing], transport)

    expect(result.consumer).toBe('triage')
    expect(result.upToOffset).toBe('2')
    expect(result.outputs).toHaveLength(2)
    expect(result.outputs).toEqual(
      expect.arrayContaining([
        { sink: { kind: 'feed', targetId: 'triage/team-feed' }, key: '1', payload: { repo: 'acme/app' }, unread: true },
        { sink: { kind: 'action', targetId: 'review-pr' }, key: '1', payload: { repo: 'acme/app' }, unread: false },
      ]),
    )
    expect(result.discards).toEqual([{ msgId: '2', nodeId: 'drop-bots' }])

    const filterRun = result.nodeRuns.find((nr) => nr.nodeId === 'drop-bots')
    expect(filterRun).toMatchObject({ flowId: 'triage', nodeId: 'drop-bots', ok: true, inCount: 2, outCount: 1, dropCount: 1 })
  })

  it('a msg matching no entry node topic is discarded as unrouted (still accounted for)', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const flow: Flow = {
      id: 'f',
      nodes: [{ id: 'src', type: 'github-source', config: { source: 'team-prs' } }],
      wires: [],
    }
    const result = await runGraph(flow, [msg('1', {}, 'source:something-else')], transport)
    expect(result.discards).toEqual([{ msgId: '1', nodeId: UNROUTED_NODE_ID }])
  })

  it('an ordinary thrown error marks the node errored and discards the msg, without resetting the transport', async () => {
    const resetSpy = vi.fn()
    const failingTransport: WorkerTransport = {
      run: async () => {
        throw new Error('boom')
      },
      reset: resetSpy,
    }
    const flow: Flow = { id: 'f', nodes: [{ id: 'n', type: 'function', config: { on_message: 'return msg' } }], wires: [] }
    const result = await runGraph(flow, [msg('1')], failingTransport)
    expect(result.discards).toEqual([{ msgId: '1', nodeId: 'n' }])
    expect(result.nodeRuns.find((r) => r.nodeId === 'n')).toMatchObject({ ok: false, err: 'boom' })
    expect(resetSpy).not.toHaveBeenCalled()
  })

  it('a transport timeout errors the node, discards the msg, and calls transport.reset', async () => {
    const resetSpy = vi.fn()
    const timeoutTransport: WorkerTransport = {
      run: async () => {
        throw new NodeTimeoutError(10)
      },
      reset: resetSpy,
    }
    const flow: Flow = { id: 'flow-timeout', nodes: [{ id: 'slow', type: 'function', config: { on_message: 'return msg' } }], wires: [] }
    const result = await runGraph(flow, [msg('1')], timeoutTransport)
    expect(result.discards).toEqual([{ msgId: '1', nodeId: 'slow' }])
    const nodeRun = result.nodeRuns.find((r) => r.nodeId === 'slow')
    expect(nodeRun?.ok).toBe(false)
    expect(nodeRun?.err).toMatch(/did not complete/)
    expect(resetSpy).toHaveBeenCalledWith('flow-timeout:slow')
  })

  it('upToOffset is the max msg id across an empty or non-empty batch', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const flow: Flow = { id: 'f', nodes: [], wires: [] }
    const empty = await runGraph(flow, [], transport)
    expect(empty.upToOffset).toBe('0')
    const some = await runGraph(flow, [msg('5'), msg('3'), msg('9')], transport)
    expect(some.upToOffset).toBe('9')
  })
})
