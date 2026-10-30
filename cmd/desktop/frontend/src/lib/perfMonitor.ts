import { nextTick } from 'vue'
import type { Router } from 'vue-router'
import { resolvePerfEnabled, usePerf } from '../composables/usePerf'
import { collectFrames, percentile, round } from './frameCollector'

const perf = usePerf('ui')

// One span per window keeps a day of idle development to a few thousand lines.
const WINDOW_MS = 10_000
const LAG_PROBE_MS = 250
const LONG_TASK_MS = 50

/**
 * The continuous half of the frontend's profile: what the whole page does for
 * as long as recording is on, with no interaction instrumented. A shipped build
 * returns at the gate and installs nothing.
 */
export async function startPerfMonitor(router?: Router): Promise<void> {
  if (!(await resolvePerfEnabled())) return
  watchWindows()
  watchLongTasks()
  watchRequests()
  if (router) watchRoutes(router)
  void import('./perf/terminalBench').then(({ terminalBench }) => {
    ;(window as unknown as { __hiveTerminalBench: unknown }).__hiveTerminalBench = terminalBench
  })
}

function watchWindows(): void {
  let lags: number[] = []
  let expected = performance.now() + LAG_PROBE_MS
  let stopFrames: ReturnType<typeof collectFrames> | null = null
  let openedAt = 0

  function open(): void {
    stopFrames = collectFrames()
    openedAt = performance.now()
    lags = []
    expected = openedAt + LAG_PROBE_MS
  }

  function close(): void {
    if (!stopFrames) return
    const frames = stopFrames()
    stopFrames = null
    lags.sort((a, b) => a - b)
    perf.record('window', performance.now() - openedAt, {
      ...frames,
      lagP50: round(percentile(lags, 50)),
      lagP95: round(percentile(lags, 95)),
      lagWorst: round(lags[lags.length - 1] ?? 0),
      heapMb: heapMegabytes(),
    })
  }

  setInterval(() => {
    const now = performance.now()
    if (stopFrames) lags.push(Math.max(0, now - expected))
    expected = now + LAG_PROBE_MS
    if (stopFrames && now - openedAt >= WINDOW_MS) {
      close()
      open()
    }
  }, LAG_PROBE_MS)

  // A hidden webview never fires requestAnimationFrame, so a window spanning it
  // would read as one enormous stall that is really the OS declining to draw.
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) close()
    else open()
  })
  if (!document.hidden) open()
}

function heapMegabytes(): number | undefined {
  const memory = (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory
  return memory ? round(memory.usedJSHeapSize / (1 << 20)) : undefined
}

// Chromium only: WebKit has neither entry type, which is why window lag exists.
function watchLongTasks(): void {
  observe('long-animation-frame', (entry) => {
    const frame = entry as PerformanceEntry & {
      blockingDuration?: number
      scripts?: { invoker?: string; sourceFunctionName?: string; duration?: number }[]
    }
    const worst = [...(frame.scripts ?? [])].sort((a, b) => (b.duration ?? 0) - (a.duration ?? 0))[0]
    perf.record('frame:long-animation', frame.duration, {
      blockingMs: round(frame.blockingDuration ?? 0),
      invoker: worst?.invoker?.slice(0, 200),
      fn: worst?.sourceFunctionName?.slice(0, 200),
    })
  })
  observe('longtask', (entry) => {
    if (entry.duration >= LONG_TASK_MS) perf.record('task:long', entry.duration)
  })
}

function watchRequests(): void {
  observe('resource', (entry) => {
    const { pathname } = new URL(entry.name, window.location.href)
    if (!pathname.startsWith('/api/terminal/')) return
    perf.record('request', entry.duration, { path: pathname })
  })
}

function observe(type: string, record: (entry: PerformanceEntry) => void): void {
  try {
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) record(entry)
    }).observe({ type, buffered: false })
  } catch {
    // An entry type this engine does not have.
  }
}

function watchRoutes(router: Router): void {
  let end: ReturnType<typeof perf.start> | null = null
  router.beforeEach((to, from) => {
    end = perf.start('route:change', { from: String(from.name ?? ''), to: String(to.name ?? '') })
  })
  // To the frame after Vue flushed the new view: what the person actually waits for.
  router.afterEach(() => {
    const close = end
    end = null
    // A covered window fires no frames, so a span closed by one measures the OS.
    if (document.hidden) return
    void nextTick(() => requestAnimationFrame(() => close?.()))
  })
}
