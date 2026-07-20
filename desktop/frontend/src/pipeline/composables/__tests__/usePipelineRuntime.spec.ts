import { describe, expect, it, vi } from 'vitest'
import { usePipelineRuntime } from '../usePipelineRuntime'
import type { PipelineClient } from '../../driver'
import type { WorkerTransport } from '../../engine/transport'
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
  it('run() drains a backlog larger than one 500-row page before returning', async () => {
    const backlog = Array.from({ length: 501 }, (_, index) => msg(String(index + 1)))
    const readFrom = vi.fn()
      .mockResolvedValueOnce(backlog.slice(0, 500))
      .mockResolvedValueOnce(backlog.slice(500))
      .mockResolvedValueOnce([])
    const commit = vi.fn().mockResolvedValue(undefined)
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    await runtime.run()

    expect(readFrom).toHaveBeenCalledTimes(3)
    expect(readFrom).toHaveBeenNthCalledWith(1, 'flow-1', 500)
    expect(readFrom).toHaveBeenNthCalledWith(2, 'flow-1', 500)
    expect(commit).toHaveBeenCalledTimes(2)
    expect(commit).toHaveBeenLastCalledWith(expect.objectContaining({ upToOffset: '501' }))
    expect(runtime.lastRun.value).toMatchObject({ batchSize: 1, outputCount: 1 })
    expect(runtime.offset.value).toBe('501')
  })

  it('latches a wakeup received during an empty read and drains the newly available page', async () => {
    let resolveInitialRead!: (value: Msg[]) => void
    const readFrom = vi.fn()
      .mockImplementationOnce(() => new Promise<Msg[]>((resolve) => { resolveInitialRead = resolve }))
      .mockResolvedValueOnce([msg('1')])
      .mockResolvedValueOnce([])
    const commit = vi.fn().mockResolvedValue(undefined)
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    const initialDrain = runtime.run()
    expect(runtime.pumping.value).toBe(true)
    const wakeDrain = runtime.pump()

    resolveInitialRead([])
    await Promise.all([initialDrain, wakeDrain])

    expect(readFrom).toHaveBeenCalledTimes(3)
    expect(commit).toHaveBeenCalledTimes(1)
    expect(runtime.offset.value).toBe('1')
  })

  it('stops an in-flight drain on a flow switch without starting another page', async () => {
    let resolveRead!: (value: Msg[]) => void
    const readFrom = vi.fn().mockImplementation(
      () => new Promise<Msg[]>((resolve) => { resolveRead = resolve }),
    )
    const commit = vi.fn().mockResolvedValue(undefined)
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    const drain = runtime.run()
    runtime.stop()
    resolveRead([msg('1')])
    await drain

    // Cancellation drops the returned page before graph execution/commit and
    // prevents the old flow runtime from reading any subsequent page.
    expect(readFrom).toHaveBeenCalledTimes(1)
    expect(commit).not.toHaveBeenCalled()
    expect(runtime.running.value).toBe(false)
    expect(runtime.lastRun.value).toBeNull()
  })

  it('stop() halts the runtime — a subsequent pump() call is a no-op', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn().mockResolvedValue(undefined)
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())
    await runtime.run()
    runtime.stop()
    readFrom.mockClear()

    await runtime.pump()

    expect(readFrom).not.toHaveBeenCalled()
    expect(runtime.running.value).toBe(false)
  })

  it('keeps stop() restartable but dispose() permanently releases the driver transport', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const transport: WorkerTransport = { run: vi.fn(), reset: vi.fn(), dispose: vi.fn() }
    const runtime = usePipelineRuntime({ readFrom, commit: vi.fn() }, simpleFlow(), { transport })

    await runtime.run()
    runtime.stop()
    await runtime.run()
    expect(readFrom).toHaveBeenCalledTimes(2)

    runtime.dispose()
    await runtime.run()
    expect(readFrom).toHaveBeenCalledTimes(2)
    expect(transport.dispose).toHaveBeenCalledOnce()
  })

  it('surfaces a pump failure without throwing and leaves the committed offset unset', async () => {
    const readFrom = vi.fn().mockResolvedValue([msg('1')])
    const commit = vi.fn().mockRejectedValue(new Error('commit failed'))
    const runtime = usePipelineRuntime({ readFrom, commit }, simpleFlow())

    await runtime.run()

    expect(runtime.error.value).toBe('commit failed')
    expect(runtime.offset.value).toBeNull()
    expect(runtime.pumping.value).toBe(false)
  })
})
