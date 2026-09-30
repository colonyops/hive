import { ref } from 'vue'
import { Echo, Ping } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/devtoolsservice'

/** Calls thrown away before timing starts, so a cold first call is not measured. */
const WARMUP = 5
/**
 * Calls per timed batch, and batches per leg. A round trip costs a fraction of
 * a millisecond and WebKit clamps performance.now() to about that, so timing
 * one call at a time measures the clock rather than the boundary: every sample
 * comes back 0ms or 1ms and the percentiles are quantisation noise. Timing a
 * batch puts the interval well above the clock's granularity and divides back
 * down; several batches then show the spread.
 */
const BATCH = 20
const BATCHES = 5

/** The payload leg, sized like a terminal frame rather than like a settings read. */
export const PAYLOAD_BYTES = 64 * 1024

export interface Latency {
  /** Calls behind the numbers below. */
  calls: number
  /** Per-call cost across every batch. */
  meanMs: number
  /** Per-call cost of the fastest and slowest batch. */
  minMs: number
  maxMs: number
}

export interface LatencyReport {
  /** An empty call: what a Wails round trip costs before it carries anything. */
  empty: Latency
  /** The same call carrying PAYLOAD_BYTES back, so the two differ by marshalling. */
  payload: Latency
  /** The clock's own granularity, which is why the calls are batched. */
  resolutionMs: number
}

// The smallest non-zero gap performance.now() will report here. Measured rather
// than assumed: it is 1ms in a WKWebView, finer in a cross-origin-isolated
// context, and the panel should say which it got.
export function timerResolutionMs(iterations = 20_000): number {
  let smallest = Number.POSITIVE_INFINITY
  let previous = performance.now()
  for (let i = 0; i < iterations; i++) {
    const now = performance.now()
    if (now > previous) {
      smallest = Math.min(smallest, now - previous)
      previous = now
    }
  }
  return Number.isFinite(smallest) ? smallest : 0
}

// Calls are issued one at a time on purpose: firing them together would measure
// how deep the queue got, not what a round trip costs.
async function time(call: () => Promise<unknown>): Promise<Latency> {
  for (let i = 0; i < WARMUP; i++) await call()

  const perCall: number[] = []
  for (let batch = 0; batch < BATCHES; batch++) {
    const started = performance.now()
    for (let i = 0; i < BATCH; i++) await call()
    perCall.push((performance.now() - started) / BATCH)
  }

  return {
    calls: BATCH * BATCHES,
    meanMs: perCall.reduce((total, ms) => total + ms, 0) / perCall.length,
    minMs: Math.min(...perCall),
    maxMs: Math.max(...perCall),
  }
}

// useWailsLatency prices the frontend↔Go boundary from the side that pays for
// it. Both legs go through the same bound-method plumbing every service call
// uses, so the numbers are the app's own, not a synthetic benchmark's.
export function useWailsLatency() {
  const report = ref<LatencyReport | null>(null)
  const measuring = ref(false)
  const error = ref('')

  async function measure(): Promise<void> {
    if (measuring.value) return
    measuring.value = true
    error.value = ''
    try {
      const resolutionMs = timerResolutionMs()
      const empty = await time(() => Ping())
      const payload = await time(() => Echo(PAYLOAD_BYTES))
      report.value = { empty, payload, resolutionMs }
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
    } finally {
      measuring.value = false
    }
  }

  return { report, measuring, error, measure }
}
