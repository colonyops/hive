import { describe, expect, it } from 'vitest'
import { summarize } from '../useFrameStats'

const PERIOD_60HZ = 1000 / 60

function frames(now: number, durations: number[]) {
  // Laid back from `now`, newest last, as the sampler collects them.
  let at = now
  return [...durations].reverse().map((ms) => {
    at -= ms
    return { at: at + ms, ms }
  }).reverse()
}

describe('summarize', () => {
  it('reports the frame rate the main thread actually serviced', () => {
    const stats = summarize(frames(10_000, Array(60).fill(PERIOD_60HZ)), [], 10_000, PERIOD_60HZ)

    expect(stats.fps).toBeCloseTo(60, 5)
    expect(stats.frameMs).toBeCloseTo(PERIOD_60HZ, 5)
    expect(stats.dropped).toBe(0)
  })

  // The whole point of the number: a stall the mean would hide.
  it('counts a missed budget as dropped and keeps the worst frame', () => {
    const stats = summarize(frames(10_000, [16, 16, 200, 16, 16]), [], 10_000, PERIOD_60HZ)

    expect(stats.dropped).toBe(1)
    expect(stats.worstFrameMs).toBe(200)
  })

  it('buckets by probe tick and keeps each tick worst, so a stall survives the line', () => {
    const stats = summarize(
      [{ at: 9_100, ms: 16 }, { at: 9_180, ms: 240 }, { at: 9_900, ms: 16 }],
      [],
      10_000,
      PERIOD_60HZ,
    )

    expect(stats.buckets).toHaveLength(40)
    expect(Math.max(...stats.buckets)).toBe(240)
    // The two samples inside one tick collapse to that tick's worst, not their mean.
    expect(stats.buckets.filter((ms) => ms > 0)).toEqual([240, 16])
  })

  it('reports lag as a median with the worst alongside', () => {
    const stats = summarize([], [
      { at: 9_000, ms: 0 },
      { at: 9_250, ms: 2 },
      { at: 9_500, ms: 4 },
      { at: 9_750, ms: 180 },
    ], 10_000, PERIOD_60HZ)

    expect(stats.lagMs).toBe(2)
    expect(stats.worstLagMs).toBe(180)
  })

  it('has nothing to say before the first sample', () => {
    const stats = summarize([], [], 0, PERIOD_60HZ)

    expect(stats.fps).toBe(0)
    expect(stats.dropped).toBe(0)
    expect(stats.buckets).toEqual([])
  })

  // A 120Hz display drops frames at 16ms; a 60Hz one is meeting its budget.
  it('measures dropped frames against the display period it was given', () => {
    const samples = frames(10_000, [8, 8, 20, 8])

    expect(summarize(samples, [], 10_000, 1000 / 120).dropped).toBe(1)
    expect(summarize(samples, [], 10_000, PERIOD_60HZ).dropped).toBe(0)
  })
})
