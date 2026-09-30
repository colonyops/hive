import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { PAYLOAD_BYTES, timerResolutionMs, useWailsLatency } from '../useWailsLatency'

const mocks = vi.hoisted(() => ({ Ping: vi.fn(), Echo: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/devtoolsservice', () => ({
  Ping: mocks.Ping,
  Echo: mocks.Echo,
}))

// A clock that only moves in whole milliseconds, like the one a WKWebView
// exposes. Timing a single sub-millisecond call against it yields 0 or 1 and
// nothing in between, which is the whole reason the calls are batched.
function coarseClock(msPerCall: number) {
  let elapsed = 0
  return {
    advanceOneCall: () => { elapsed += msPerCall },
    now: () => Math.floor(elapsed),
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useWailsLatency', () => {
  it('recovers a sub-millisecond per-call cost from a clock that cannot resolve one', async () => {
    const clock = coarseClock(0.4)
    vi.spyOn(performance, 'now').mockImplementation(clock.now)
    mocks.Ping.mockImplementation(async () => { clock.advanceOneCall() })
    mocks.Echo.mockImplementation(async () => { clock.advanceOneCall() })

    const scope = effectScope()
    const { report, measure } = scope.run(() => useWailsLatency())!
    await measure()

    // 20 calls at 0.4ms is an 8ms batch — well clear of the 1ms floor.
    expect(report.value?.empty.meanMs).toBeCloseTo(0.4, 1)
    expect(report.value?.empty.calls).toBe(100)
    expect(mocks.Echo).toHaveBeenCalledWith(PAYLOAD_BYTES)
    scope.stop()
  })

  it('issues a warm-up per leg that no timing counts', async () => {
    mocks.Ping.mockResolvedValue(0)
    mocks.Echo.mockResolvedValue('x')

    const scope = effectScope()
    const { report, measure } = scope.run(() => useWailsLatency())!
    await measure()

    // 5 warm-up calls plus 5 batches of 20, per leg.
    expect(mocks.Ping).toHaveBeenCalledTimes(105)
    expect(mocks.Echo).toHaveBeenCalledTimes(105)
    expect(report.value?.empty.calls).toBe(100)
    scope.stop()
  })

  it('reports the clock granularity it had to work around', async () => {
    let ticks = 0
    vi.spyOn(performance, 'now').mockImplementation(() => ++ticks)

    const scope = effectScope()
    const { report, measure } = scope.run(() => useWailsLatency())!
    await measure()

    expect(report.value?.resolutionMs).toBe(1)
    scope.stop()
  })

  it('surfaces a failed call rather than reporting a time for it', async () => {
    mocks.Ping.mockRejectedValue(new Error('no runtime'))

    const scope = effectScope()
    const { report, error, measure } = scope.run(() => useWailsLatency())!
    await measure()

    expect(error.value).toContain('no runtime')
    expect(report.value).toBeNull()
    scope.stop()
  })
})

describe('timerResolutionMs', () => {
  it('finds the smallest gap the clock will report', () => {
    let ticks = 0
    vi.spyOn(performance, 'now').mockImplementation(() => (ticks += 0.25))
    expect(timerResolutionMs(100)).toBeCloseTo(0.25, 5)
  })
})
