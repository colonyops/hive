import { shallowRef } from 'vue'
import { usePerf } from './usePerf'

/** How much history the rolling figures cover. */
const WINDOW_MS = 10_000
/** How often the event-loop probe is scheduled; how late it runs is the lag. */
const PROBE_MS = 250
/** One bucket per probe tick, so the frame line is 10s wide at SparkLine's slot count. */
const BUCKETS = WINDOW_MS / PROBE_MS
/** A frame at or past this multiple of the display's own period missed its budget. */
const DROPPED_AT = 2
/** The period assumed until the display has shown its hand. */
const FALLBACK_PERIOD_MS = 1000 / 60
/** Past these, a sample is worth a span in perf.jsonl rather than just a number on screen. */
const LAG_SPAN_MS = 50

interface Sample {
  at: number
  ms: number
}

export interface FrameStats {
  /** Frames the main thread serviced per second, over the window. */
  fps: number
  /** Mean and worst frame in the window. */
  frameMs: number
  worstFrameMs: number
  /** Frames that missed their budget outright, and the window they missed it in. */
  dropped: number
  windowMs: number
  /** Event-loop lag: how late a scheduled callback actually ran. */
  lagMs: number
  worstLagMs: number
  /** Worst frame per probe tick, oldest first — the shape a sparkline wants. */
  buckets: number[]
}

const EMPTY: FrameStats = {
  fps: 0,
  frameMs: 0,
  worstFrameMs: 0,
  dropped: 0,
  windowMs: 0,
  lagMs: 0,
  worstLagMs: 0,
  buckets: [],
}

// Summarised from plain arrays rather than reactive ones: frames arrive up to
// 120 times a second and a reactive push per frame would put Vue's dependency
// tracking in the path of the very thing being measured. One write per probe
// tick instead.
const stats = shallowRef<FrameStats>(EMPTY)
const running = shallowRef(false)

let frames: Sample[] = []
let lags: Sample[] = []
// The fastest interval the display has produced, which is its refresh period —
// nothing can render faster than the panel refreshes.
let periodMs = FALLBACK_PERIOD_MS
let lastFrameAt = 0
let expectedProbeAt = 0
let rafHandle = 0
let probe: ReturnType<typeof setInterval> | undefined

const perf = usePerf('ui')

function prune(samples: Sample[], now: number): Sample[] {
  const cutoff = now - WINDOW_MS
  const first = samples.findIndex((sample) => sample.at >= cutoff)
  return first <= 0 ? samples : samples.slice(first)
}

function percentile(values: number[], p: number): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  return sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil((p / 100) * sorted.length) - 1))]
}

/**
 * Roll the raw samples up into what the panel shows. Pure and exported so the
 * arithmetic can be tested without a display attached.
 */
export function summarize(frameSamples: Sample[], lagSamples: Sample[], now: number, displayPeriodMs: number): FrameStats {
  if (frameSamples.length === 0 && lagSamples.length === 0) return EMPTY

  const durations = frameSamples.map((sample) => sample.ms)
  const total = durations.reduce((sum, ms) => sum + ms, 0)
  const mean = durations.length ? total / durations.length : 0
  const lagValues = lagSamples.map((sample) => sample.ms)

  // One bucket per probe tick, holding that tick's worst frame: a mean would
  // average away the single 200ms stall that is the whole point of the line.
  const newest = Math.ceil(now / PROBE_MS)
  const buckets = new Array<number>(BUCKETS).fill(0)
  for (const sample of frameSamples) {
    const index = BUCKETS - 1 - (newest - Math.ceil(sample.at / PROBE_MS))
    if (index >= 0 && index < BUCKETS) buckets[index] = Math.max(buckets[index], sample.ms)
  }

  return {
    fps: mean > 0 ? 1000 / mean : 0,
    frameMs: mean,
    worstFrameMs: durations.length ? Math.max(...durations) : 0,
    dropped: durations.filter((ms) => ms >= displayPeriodMs * DROPPED_AT).length,
    windowMs: WINDOW_MS,
    lagMs: percentile(lagValues, 50),
    worstLagMs: lagValues.length ? Math.max(...lagValues) : 0,
    buckets,
  }
}

function onFrame(now: number): void {
  if (lastFrameAt > 0) {
    const ms = now - lastFrameAt
    frames.push({ at: now, ms })
    // Sub-millisecond intervals are the clock's clamping, not a 2000Hz display.
    if (ms >= 1 && ms < periodMs) periodMs = ms
    if (ms >= periodMs * DROPPED_AT) perf.record('frame:long', ms)
  }
  lastFrameAt = now
  rafHandle = requestAnimationFrame(onFrame)
}

function onProbe(): void {
  const now = performance.now()
  if (expectedProbeAt > 0) {
    const ms = Math.max(0, now - expectedProbeAt)
    lags.push({ at: now, ms })
    if (ms >= LAG_SPAN_MS) perf.record('loop:lag', ms)
  }
  // Re-anchored to now rather than advanced by the interval, so one long stall
  // is reported once instead of cascading into every tick behind it.
  expectedProbeAt = now + PROBE_MS

  frames = prune(frames, now)
  lags = prune(lags, now)
  stats.value = summarize(frames, lags, now, periodMs)
}

function attach(): void {
  if (typeof requestAnimationFrame !== 'function') return
  lastFrameAt = 0
  expectedProbeAt = performance.now() + PROBE_MS
  rafHandle = requestAnimationFrame(onFrame)
  probe = setInterval(onProbe, PROBE_MS)
}

function detach(): void {
  if (rafHandle) cancelAnimationFrame(rafHandle)
  rafHandle = 0
  if (probe !== undefined) clearInterval(probe)
  probe = undefined
  // Dropped so the gap across a pause is not counted as one enormous frame.
  lastFrameAt = 0
  expectedProbeAt = 0
}

// requestAnimationFrame stops while the window is occluded, so an unguarded
// sampler would report a stall that is really the OS declining to draw.
function onVisibility(): void {
  if (!running.value) return
  document.hidden ? detach() : attach()
}

// The dashboard scopes this requestAnimationFrame loop to its mount lifecycle
// so the webview can idle after the page closes.
export function startFrameStats(): void {
  if (running.value) return
  running.value = true
  document.addEventListener('visibilitychange', onVisibility)
  if (!document.hidden) attach()
}

export function stopFrameStats(): void {
  if (!running.value) return
  running.value = false
  document.removeEventListener('visibilitychange', onVisibility)
  detach()
  frames = []
  lags = []
  periodMs = FALLBACK_PERIOD_MS
  stats.value = EMPTY
}

export function useFrameStats() {
  return { stats, running }
}
