import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPerf, resetPerfForTests, usePerf } from '../usePerf'

const mocks = vi.hoisted(() => ({ Info: vi.fn(), Record: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/perfservice', () => ({
  Info: mocks.Info,
  Record: mocks.Record,
}))

function recorded(call = 0): Array<Record<string, unknown>> {
  return mocks.Record.mock.calls[call][0]
}

beforeEach(() => {
  vi.clearAllMocks()
  resetPerfForTests()
  mocks.Info.mockResolvedValue({ enabled: true, path: '/state/perf.jsonl', maxBytes: 8 << 20 })
  mocks.Record.mockResolvedValue({ written: 0 })
})

afterEach(() => {
  vi.useRealTimers()
  resetPerfForTests()
})

describe('usePerf', () => {
  it('batches samples into one call rather than one per sample', async () => {
    const perf = usePerf('feed')

    perf.record('item:open', 12.5)
    perf.record('item:close', 3)
    await flushPerf()

    expect(mocks.Record).toHaveBeenCalledTimes(1)
    const batch = recorded()
    expect(batch).toHaveLength(2)
    expect(batch[0]).toMatchObject({ scope: 'feed', name: 'item:open', durationMs: 12.5 })
    expect(batch[1]).toMatchObject({ scope: 'feed', name: 'item:close', durationMs: 3 })
  })

  it('sends nothing when the backend reports recording is off', async () => {
    mocks.Info.mockResolvedValue({ enabled: false, path: '', maxBytes: 0 })
    const perf = usePerf('feed')

    perf.record('item:open', 1)
    await flushPerf()

    expect(mocks.Record).not.toHaveBeenCalled()
  })

  it('stops buffering once the gate has answered off', async () => {
    mocks.Info.mockResolvedValue({ enabled: false, path: '', maxBytes: 0 })
    const perf = usePerf('feed')

    perf.record('first', 1)
    await flushPerf()
    perf.record('second', 1)
    await flushPerf()

    expect(mocks.Info).toHaveBeenCalledTimes(1)
    expect(mocks.Record).not.toHaveBeenCalled()
  })

  it('measures elapsed time for a started span and merges attrs from both ends', async () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValueOnce(1000).mockReturnValueOnce(1250)
    const perf = usePerf('feed')

    const end = perf.start('item:open', { itemId: 'i1' })
    end({ rendered: true })
    await flushPerf()

    expect(recorded()[0]).toMatchObject({
      scope: 'feed',
      name: 'item:open',
      durationMs: 250,
      attrs: { itemId: 'i1', rendered: true },
    })
  })

  it('ignores a second call to the same span closer', async () => {
    const perf = usePerf('feed')

    const end = perf.start('item:open')
    end()
    end()
    await flushPerf()

    expect(recorded()).toHaveLength(1)
  })

  it('passes a tracked result through and records the span', async () => {
    const perf = usePerf('feed')

    const result = await perf.track('fetch', async () => 'payload', { repo: 'o/r' })

    expect(result).toBe('payload')
    await flushPerf()
    expect(recorded()[0]).toMatchObject({ name: 'fetch', attrs: { repo: 'o/r' } })
  })

  it('records a failed track and rethrows', async () => {
    const perf = usePerf('feed')

    await expect(perf.track('fetch', () => Promise.reject(new Error('boom')))).rejects.toThrow('boom')

    await flushPerf()
    expect(recorded()[0]).toMatchObject({ name: 'fetch', attrs: { failed: true } })
  })

  it('flushes on the interval without an explicit call', async () => {
    vi.useFakeTimers()
    const perf = usePerf('feed')

    perf.record('item:open', 1)
    expect(mocks.Record).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(2000)

    expect(mocks.Record).toHaveBeenCalledTimes(1)
  })

  it('flushes early once the buffer fills rather than growing unbounded', async () => {
    vi.useFakeTimers()
    const perf = usePerf('feed')

    for (let i = 0; i < 256; i++) perf.record(`item:${i}`, 1)
    await vi.advanceTimersByTimeAsync(0)

    expect(mocks.Record).toHaveBeenCalledTimes(1)
    expect(recorded()).toHaveLength(256)
  })

  it('keeps buffering while the gate is still resolving', async () => {
    let resolveInfo: (info: unknown) => void = () => {}
    mocks.Info.mockReturnValue(
      new Promise((resolve) => {
        resolveInfo = resolve
      }),
    )
    const perf = usePerf('feed')

    perf.record('startup', 5)
    const flushing = flushPerf()
    resolveInfo({ enabled: true, path: '/state/perf.jsonl', maxBytes: 1 })
    await flushing

    expect(recorded()[0]).toMatchObject({ name: 'startup' })
  })

  it('swallows a rejected batch so instrumentation cannot break the caller', async () => {
    mocks.Record.mockRejectedValue(new Error('backend gone'))
    const perf = usePerf('feed')

    perf.record('item:open', 1)

    await expect(flushPerf()).resolves.toBeUndefined()
  })
})
