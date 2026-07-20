import { describe, expect, it, vi } from 'vitest'
import { WebWorkerTransport, type WorkerFactory, type WorkerLike } from '../webWorkerTransport'
import { NodeTimeoutError } from '../transport'
import type { Msg } from '../../types'

function msg(id: string): Msg {
  return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: {} }
}

// A fake WorkerLike: echoes the request's msg back as the result and merges
// {touched:true} onto its (structured-clone) copy of state, exactly the
// protocol WebWorkerTransport expects a real worker script to speak.
// respond:false simulates a stuck worker for the timeout tests.
class FakeWorker implements WorkerLike {
  onmessage: ((ev: { data: any }) => void) | null = null
  onerror: ((ev: any) => void) | null = null
  terminated = false
  terminate = vi.fn(() => {
    this.terminated = true
  })

  constructor(private readonly respond: boolean) {}

  postMessage(data: any): void {
    if (!this.respond) return
    queueMicrotask(() => {
      if (this.terminated) return
      this.onmessage?.({ data: { id: data.id, result: data.msg, state: { ...data.state, touched: true } } })
    })
  }
}

function fakeFactory(respond: boolean | ((index: number) => boolean) = true): { factory: WorkerFactory; workers: FakeWorker[] } {
  const workers: FakeWorker[] = []
  const factory: WorkerFactory = () => {
    const worker = new FakeWorker(typeof respond === 'function' ? respond(workers.length) : respond)
    workers.push(worker)
    return worker
  }
  return { factory, workers }
}

describe('WebWorkerTransport', () => {
  it('runs a request through the injected worker and resolves with its result', async () => {
    const { factory } = fakeFactory()
    const transport = new WebWorkerTransport(factory)
    const m = msg('1')
    const result = await transport.run('github-filter', 'flow:filter', {}, m, {}, 1000)
    expect(result).toBe(m)
  })

  it("merges the worker-returned state back onto the caller's original state object", async () => {
    const { factory } = fakeFactory()
    const transport = new WebWorkerTransport(factory)
    const state = { seen: 1 }
    await transport.run('github-filter', 'flow:filter', {}, msg('1'), state, 1000)
    expect(state).toEqual({ seen: 1, touched: true })
  })

  it('hosts every isolate:false instance on one shared worker', async () => {
    const { factory, workers } = fakeFactory()
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await transport.run('github-filter', 'flow:a', {}, msg('1'), {}, 1000)
    await transport.run('github-filter', 'flow:b', {}, msg('2'), {}, 1000)
    expect(workers.length).toBe(1)
  })

  it('spawns a dedicated worker per isolate:true instance', async () => {
    const { factory, workers } = fakeFactory()
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await transport.run('function', 'flow:a', {}, msg('1'), {}, 1000)
    await transport.run('function', 'flow:b', {}, msg('2'), {}, 1000)
    expect(workers.length).toBe(2)
  })

  it('reuses the same dedicated worker across calls for the same instance', async () => {
    const { factory, workers } = fakeFactory()
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await transport.run('function', 'flow:a', {}, msg('1'), {}, 1000)
    await transport.run('function', 'flow:a', {}, msg('2'), {}, 1000)
    expect(workers.length).toBe(1)
  })

  it('terminates the dedicated worker and rejects with NodeTimeoutError on timeout', async () => {
    const { factory, workers } = fakeFactory(false) // never responds
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await expect(transport.run('function', 'flow:stuck', {}, msg('1'), {}, 10)).rejects.toBeInstanceOf(NodeTimeoutError)
    expect(workers[0].terminate).toHaveBeenCalled()
  })

  it('terminates and replaces the shared worker on timeout so poisoned state cannot persist', async () => {
    const { factory, workers } = fakeFactory(false)
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await expect(transport.run('github-filter', 'flow:stuck', {}, msg('1'), {}, 10)).rejects.toBeInstanceOf(NodeTimeoutError)
    expect(workers[0].terminate).toHaveBeenCalled()
    expect(workers).toHaveLength(2)
  })

  it('terminates and replaces an isolated worker before the next function request', async () => {
    const { factory, workers } = fakeFactory((index) => index !== 0)
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await expect(transport.run('function', 'flow:stuck', {}, msg('1'), {}, 10)).rejects.toBeInstanceOf(NodeTimeoutError)
    expect(workers[0].terminate).toHaveBeenCalledOnce()
    expect(workers).toHaveLength(2)

    const second = msg('2')
    await expect(transport.run('function', 'flow:stuck', {}, second, {}, 1000)).resolves.toBe(second)
  })

  it('reset() terminates the dedicated worker so the next run spawns a fresh one', async () => {
    const { factory, workers } = fakeFactory()
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await transport.run('function', 'flow:a', {}, msg('1'), {}, 1000)
    transport.reset('flow:a')
    expect(workers[0].terminate).toHaveBeenCalled()
    await transport.run('function', 'flow:a', {}, msg('2'), {}, 1000)
    expect(workers.length).toBe(2)
  })

  it('terminates the shared worker and every isolated worker on disposal', async () => {
    const { factory, workers } = fakeFactory()
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    await transport.run('github-filter', 'flow:filter', {}, msg('1'), {}, 1000)
    await transport.run('function', 'flow:a', {}, msg('2'), {}, 1000)
    await transport.run('function', 'flow:b', {}, msg('3'), {}, 1000)

    transport.dispose()

    expect(workers).toHaveLength(3)
    for (const worker of workers) {
      expect(worker.terminate).toHaveBeenCalledOnce()
      expect(worker.onmessage).toBeNull()
      expect(worker.onerror).toBeNull()
    }
    await expect(transport.run('function', 'flow:c', {}, msg('4'), {}, 1000)).rejects.toThrow('disposed')
  })

  it('rejects in-flight work when disposed', async () => {
    const { factory, workers } = fakeFactory(false)
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    const pending = transport.run('function', 'flow:a', {}, msg('1'), {}, 5000)

    transport.dispose()

    await expect(pending).rejects.toThrow('disposed')
    expect(workers[0].terminate).toHaveBeenCalledOnce()
  })

  it('rejects still-pending requests when the worker reports an error', async () => {
    const { factory, workers } = fakeFactory(false)
    const transport = new WebWorkerTransport(factory, new Set(['function']))
    const pending = transport.run('function', 'flow:a', {}, msg('1'), {}, 5000)
    workers[0].onerror?.({ message: 'boom' })
    await expect(pending).rejects.toThrow('boom')
  })
})
