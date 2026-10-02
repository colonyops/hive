import {
  Info,
  Record as RecordSamples,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/perfservice'
import type { PerfSample } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const FLUSH_INTERVAL_MS = 2000
const MAX_BUFFERED = 256

// Resolved once from the backend. Until it answers, samples buffer: the gate is
// off in a shipped build, and an interaction measured during startup is exactly
// the kind this exists to catch, so dropping it would defeat the purpose.
let enabled: boolean | null = null
let infoRequest: Promise<boolean> | null = null

let buffer: PerfSample[] = []
let timer: ReturnType<typeof setTimeout> | null = null

function gate(): Promise<boolean> {
  if (enabled !== null) return Promise.resolve(enabled)
  infoRequest ??= Info()
    .then((info) => {
      enabled = info.enabled
      return enabled
    })
    .catch(() => {
      enabled = false
      return false
    })
  return infoRequest
}

async function flush(): Promise<void> {
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
  if (buffer.length === 0) return

  if (!(await gate())) {
    buffer = []
    return
  }
  // Re-checked after the await: a concurrent flush may have taken the buffer
  // while the gate was resolving.
  if (buffer.length === 0) return

  const batch = buffer
  buffer = []
  try {
    await RecordSamples(batch)
  } catch {
    // A dropped batch is not worth surfacing: this is a development facility
    // and failing loudly here would put an error in front of whoever is
    // debugging something else entirely.
  }
}

function schedule(): void {
  if (buffer.length >= MAX_BUFFERED) {
    void flush()
    return
  }
  timer ??= setTimeout(() => void flush(), FLUSH_INTERVAL_MS)
}

// Registered on first use rather than at import so a build with recording off
// adds no listener. A reload during development is the case this covers —
// without it the last interval's samples, often the slow one being chased, go
// with the window.
let unloadBound = false
function bindUnloadFlush(): void {
  if (unloadBound || typeof window === 'undefined') return
  unloadBound = true
  window.addEventListener('pagehide', () => void flush())
}

function enqueue(sample: PerfSample): void {
  if (enabled === false) return
  bindUnloadFlush()
  buffer.push(sample)
  schedule()
}

export interface PerfScope {
  /** Record a duration measured elsewhere. */
  record(name: string, durationMs: number, attrs?: Record<string, unknown>): void
  /**
   * Open a span. The returned function closes it, recording the elapsed time;
   * attrs passed to either end are merged, so context discovered during the
   * work can still be attached.
   */
  start(name: string, attrs?: Record<string, unknown>): (extra?: Record<string, unknown>) => void
  /**
   * Time a function, sync or async. The result is passed through and a thrown
   * error is still recorded — with `failed: true` — before it propagates,
   * because a slow path that ends in an error is one worth seeing.
   */
  track<T>(name: string, fn: () => T | Promise<T>, attrs?: Record<string, unknown>): Promise<T>
}

/**
 * Performance instrumentation for one subsystem. Samples buffer in the
 * frontend and flush in batches to a JSONL file the backend owns; when
 * `development.perf.enabled` is off, every method costs a boolean check.
 *
 * Scope names a subsystem ('feed', 'terminal'); name identifies the operation
 * and should stay stable across calls so samples aggregate.
 */
export function usePerf(scope: string): PerfScope {
  function record(name: string, durationMs: number, attrs?: Record<string, unknown>): void {
    enqueue({
      atUnixMs: Date.now(),
      scope,
      name,
      durationMs,
      attrs: attrs as PerfSample['attrs'],
    })
  }

  function start(name: string, attrs?: Record<string, unknown>) {
    const startedAt = performance.now()
    const startedAtWall = Date.now()
    let closed = false
    return (extra?: Record<string, unknown>) => {
      if (closed) return
      closed = true
      const merged = attrs || extra ? { ...attrs, ...extra } : undefined
      enqueue({
        atUnixMs: startedAtWall,
        scope,
        name,
        durationMs: performance.now() - startedAt,
        attrs: merged as PerfSample['attrs'],
      })
    }
  }

  async function track<T>(name: string, fn: () => T | Promise<T>, attrs?: Record<string, unknown>): Promise<T> {
    const end = start(name, attrs)
    try {
      const result = await fn()
      end()
      return result
    } catch (err) {
      end({ failed: true })
      throw err
    }
  }

  return { record, start, track }
}

/** Send anything buffered immediately. */
export function flushPerf(): Promise<void> {
  return flush()
}

/** Where the recorder is writing, for surfacing the path to whoever is analyzing it. */
export function perfInfo() {
  return Info()
}

export function resetPerfForTests(): void {
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
  enabled = null
  infoRequest = null
  buffer = []
}
