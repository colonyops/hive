import { describe, expect, it } from 'vitest'
import { percentile, summarizeFrames } from '../frameCollector'

describe('summarizeFrames', () => {
  it('reports nothing for no samples', () => {
    expect(summarizeFrames([]).frames).toBe(0)
  })

  it('does not count normal frames as dropped because of one short interval', () => {
    const samples = [1, ...new Array<number>(99).fill(16.7)]
    expect(summarizeFrames(samples).framesDropped).toBe(0)
  })

  it('counts a frame past twice the display period', () => {
    const samples = [50, ...new Array<number>(99).fill(16.7)]
    const summary = summarizeFrames(samples)
    expect(summary.framesDropped).toBe(1)
    expect(summary.frameWorst).toBe(50)
    expect(summary.frameP50).toBe(16.7)
  })

  it('measures a 120Hz panel against its own period', () => {
    const samples = [20, ...new Array<number>(99).fill(8.3)]
    expect(summarizeFrames(samples).framesDropped).toBe(1)
  })

  it('does not let a run of only long frames excuse itself', () => {
    expect(summarizeFrames(new Array<number>(10).fill(100)).framesDropped).toBe(10)
  })
})

describe('percentile', () => {
  it('takes the nearest rank', () => {
    expect(percentile([1, 2, 3, 4], 50)).toBe(2)
    expect(percentile([1, 2, 3, 4], 95)).toBe(4)
    expect(percentile([], 50)).toBe(0)
  })
})
