import { describe, expect, it, vi } from 'vitest'
import { InProcessTransport, NodeTimeoutError, type ProcessorRuntime } from '../transport'
import type { Msg } from '../../types'

function msg(id: string, payload: any = {}): Msg {
  return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: payload, Meta: null }
}

describe('InProcessTransport', () => {
  it("runs a registered runtime's onMsg and returns its result", async () => {
    const runtime: ProcessorRuntime = { type: 'echo', onMsg: (m) => m }
    const transport = new InProcessTransport({ echo: runtime })
    const m = msg('1')
    await expect(transport.run('echo', 'flow:node', {}, m, {}, 1000)).resolves.toBe(m)
  })

  it('rejects for an unregistered runtime type', async () => {
    const transport = new InProcessTransport({})
    await expect(transport.run('missing', 'flow:node', {}, msg('1'), {}, 1000)).rejects.toThrow(/no processor runtime registered/)
  })

  it('calls start() exactly once, lazily, before the first onMsg for an instance', async () => {
    const start = vi.fn()
    const runtime: ProcessorRuntime = { type: 'counter', start, onMsg: (m) => m }
    const transport = new InProcessTransport({ counter: runtime })
    await transport.run('counter', 'inst-1', {}, msg('1'), {}, 1000)
    await transport.run('counter', 'inst-1', {}, msg('2'), {}, 1000)
    expect(start).toHaveBeenCalledTimes(1)
  })

  it('starts each instance independently', async () => {
    const start = vi.fn()
    const runtime: ProcessorRuntime = { type: 'counter', start, onMsg: (m) => m }
    const transport = new InProcessTransport({ counter: runtime })
    await transport.run('counter', 'inst-1', {}, msg('1'), {}, 1000)
    await transport.run('counter', 'inst-2', {}, msg('2'), {}, 1000)
    expect(start).toHaveBeenCalledTimes(2)
  })

  it('re-runs start() after reset()', async () => {
    const start = vi.fn()
    const runtime: ProcessorRuntime = { type: 'counter', start, onMsg: (m) => m }
    const transport = new InProcessTransport({ counter: runtime })
    await transport.run('counter', 'inst-1', {}, msg('1'), {}, 1000)
    transport.reset('inst-1')
    await transport.run('counter', 'inst-1', {}, msg('2'), {}, 1000)
    expect(start).toHaveBeenCalledTimes(2)
  })

  it('rejects with NodeTimeoutError when a slow runtime exceeds timeoutMs', async () => {
    const slow: ProcessorRuntime = {
      type: 'slow',
      onMsg: () => new Promise((resolve) => setTimeout(() => resolve(null), 50)),
    }
    const transport = new InProcessTransport({ slow })
    await expect(transport.run('slow', 'inst-1', {}, msg('1'), {}, 5)).rejects.toBeInstanceOf(NodeTimeoutError)
  })

  it('propagates an ordinary thrown error from onMsg', async () => {
    const runtime: ProcessorRuntime = {
      type: 'throws',
      onMsg: () => {
        throw new Error('kaboom')
      },
    }
    const transport = new InProcessTransport({ throws: runtime })
    await expect(transport.run('throws', 'inst-1', {}, msg('1'), {}, 1000)).rejects.toThrow('kaboom')
  })
})
