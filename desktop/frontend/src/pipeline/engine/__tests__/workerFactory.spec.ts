import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPipelineWorker, createWebWorkerTransport } from '../workerFactory'

class FakeWorker {
  onmessage: ((event: { data: any }) => void) | null = null
  onerror: ((event: any) => void) | null = null
  postMessage(): void {}
  terminate(): void {}
}

describe('pipeline worker factory', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('constructs a Vite module worker for each worker slot', () => {
    const calls: unknown[][] = []
    class WorkerMock extends FakeWorker {
      constructor(...args: unknown[]) {
        super()
        calls.push(args)
      }
    }
    vi.stubGlobal('Worker', WorkerMock)

    createPipelineWorker('isolated', 'flow:function')

    expect(calls).toHaveLength(1)
    expect(calls[0][0]).toBeInstanceOf(URL)
    expect(calls[0][1]).toEqual({ type: 'module' })
  })

  it('creates a transport that isolates function runtimes', async () => {
    const workers: FakeWorker[] = []
    class WorkerMock extends FakeWorker {
      constructor() {
        super()
        workers.push(this)
      }
    }
    vi.stubGlobal('Worker', WorkerMock)
    const transport = createWebWorkerTransport()
    const run = transport.run('function', 'flow:function', {}, { ID: '1', Key: '1', Topic: 'source:test', Ts: 0, Payload: {} }, {}, 1000)

    expect(workers).toHaveLength(1)
    workers[0].onmessage?.({ data: { id: 1, result: null, state: {} } })
    await expect(run).resolves.toBeNull()
  })
})
