export interface FrameSummary {
  frames: number
  fps: number
  frameP50: number
  frameP95: number
  frameWorst: number
  /** Frames that took at least twice the display's own period. */
  framesDropped: number
}

const FALLBACK_PERIOD_MS = 1000 / 60
const DROPPED_AT = 2

const collectors = new Set<number[]>()
let handle = 0
let lastFrameAt = 0

function onFrame(now: number): void {
  if (lastFrameAt > 0) {
    const ms = now - lastFrameAt
    for (const collector of collectors) collector.push(ms)
  }
  lastFrameAt = now
  if (collectors.size > 0) {
    handle = requestAnimationFrame(onFrame)
  } else {
    handle = 0
    lastFrameAt = 0
  }
}

/**
 * Collects frame intervals until the returned function is called. One animation
 * frame loop serves every collector, and it stops when the last one does, so a
 * probe costs nothing between the activity it measures.
 */
export function collectFrames(): () => FrameSummary {
  if (typeof requestAnimationFrame !== 'function') return () => summarizeFrames([])
  const samples: number[] = []
  collectors.add(samples)
  if (!handle) handle = requestAnimationFrame(onFrame)
  return () => {
    collectors.delete(samples)
    return summarizeFrames(samples)
  }
}

export function percentile(sorted: readonly number[], p: number): number {
  if (sorted.length === 0) return 0
  return sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil((p / 100) * sorted.length) - 1))]
}

export function summarizeFrames(samples: readonly number[]): FrameSummary {
  if (samples.length === 0) return { frames: 0, fps: 0, frameP50: 0, frameP95: 0, frameWorst: 0, framesDropped: 0 }
  const sorted = [...samples].sort((a, b) => a - b)
  const total = sorted.reduce((sum, ms) => sum + ms, 0)
  const median = percentile(sorted, 50)
  // The fastest interval is no estimate of the display: WebKit clamps its clock
  // to whole milliseconds and sometimes runs two callbacks a beat apart, and one
  // such sample would count every normal frame as dropped. The median finds a
  // 120Hz panel, and the cap keeps a run that is all long frames from excusing
  // itself.
  const period = Math.min(median, FALLBACK_PERIOD_MS)
  return {
    frames: sorted.length,
    fps: round(total > 0 ? (sorted.length * 1000) / total : 0),
    frameP50: round(median),
    frameP95: round(percentile(sorted, 95)),
    frameWorst: round(sorted[sorted.length - 1]),
    framesDropped: sorted.filter((ms) => ms >= period * DROPPED_AT).length,
  }
}

export function round(value: number): number {
  return Math.round(value * 100) / 100
}

export function resetFrameCollectorForTests(): void {
  if (handle) cancelAnimationFrame(handle)
  handle = 0
  lastFrameAt = 0
  collectors.clear()
}
