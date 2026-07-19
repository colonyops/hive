import { describe, expect, it, vi } from 'vitest'
import { usePipelineRuntime } from '../usePipelineRuntime'
import type { PipelineClient } from '../../driver'
import type { Flow, Msg } from '../../types'

function msg(id: string, payload: any = {}): Msg {
  return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: payload, Meta: null }
}

function simpleFlow(): Flow {
  return {
    id: 'flow-1',
    nodes: [{ id: 'feed', type: 'feed', config: { feed: 'inbox' } }],
    wires: [],
  }
}

describe('usePipelineRuntime', () => {
  it('run() starts the runtime and performs an initial pump: reads, runs the graph, and commits', async () => {
    const batch = [msg('1'), msg('2')]
    const readFrom = vi.fn().mockResolvedValue(batch)
    const commit = vi.fn().mockResolvedValue(undefined)
    const client: PipelineClient = { readFrom, commit }
    const runtime = usePipelineRuntime(client, simpleFlow())

    await runtime.run()

    expect(runtime.running.value).toBe(true)
    expect(readFrom).toHaveBeenCalledWith(0, 500)
    expect(commit).toHaveBeenCalledTimes(1)
    expect(runtime.lastRun.value).toEqual({
      batchSize: 2,
      outputCount: 2,
      discardCount: 0,
      errorCount: 0,
      completedAt: expect.any(Number),
    })
    expect(runtime.offset.value).toBe(2)
    expect(runtime.error.value).toBeNull()
  })

  it('stop() halts the runtime — a subsequent pump() call is a no-op', async () => {
    const readFrom = vi.fn().mockResolvedValue([msg('1')])
    const commit = vi.fn().mockResolvedValue(undefined)
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())
    await runtime.run()
    runtime.stop()
    readFrom.mockClear()
    commit.mockClear()

    await runtime.pump()

    expect(readFrom).not.toHaveBeenCalled()
    expect(commit).not.toHaveBeenCalled()
    expect(runtime.running.value).toBe(false)
  })

  it('run() is idempotent while already running — calling it twice does not double-pump', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn()
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    await runtime.run()
    readFrom.mockClear()
    await runtime.run()

    expect(readFrom).not.toHaveBeenCalled()
  })

  it('guards against overlapping pumps: a pump already in flight blocks a concurrent one', async () => {
    let resolveRead!: (value: Msg[]) => void
    const readFrom = vi.fn().mockImplementation(
      () => new Promise<Msg[]>((resolve) => { resolveRead = resolve }),
    )
    const commit = vi.fn().mockResolvedValue(undefined)
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    const runPromise = runtime.run() // starts the initial pump, blocked on readFrom
    expect(runtime.pumping.value).toBe(true)

    await runtime.pump() // a pump is already in flight — this must be a clean no-op
    expect(readFrom).toHaveBeenCalledTimes(1)

    resolveRead([msg('1')])
    await runPromise

    expect(runtime.pumping.value).toBe(false)
    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('an empty log is a clean no-op: no commit, batchSize 0, cursor unchanged', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn()
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    await runtime.run()

    expect(commit).not.toHaveBeenCalled()
    expect(runtime.lastRun.value).toEqual({
      batchSize: 0,
      outputCount: 0,
      discardCount: 0,
      errorCount: 0,
      completedAt: expect.any(Number),
    })
    expect(runtime.offset.value).toBe(0)
  })

  it('surfaces a pump failure (e.g. Commit rejecting) without throwing, and leaves the cursor unchanged', async () => {
    const readFrom = vi.fn().mockResolvedValue([msg('1')])
    const commit = vi.fn().mockRejectedValue(new Error('commit failed'))
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    await runtime.run()

    expect(runtime.error.value).toBe('commit failed')
    expect(runtime.offset.value).toBe(0)
    expect(runtime.pumping.value).toBe(false)
  })
})
