// Mounts a consumer-driven PipelineDriver for one Flow and drains its log
// whenever the caller signals that work may be available. Event subscription
// remains with App.vue so this composable stays binding-free and testable.
import { computed, ref } from 'vue'
import { PipelineDriver, type PipelineClient } from '../driver'
import type { Flow } from '../types'
import type { WorkerTransport } from '../engine/transport'

export interface PipelineRuntimeOptions {
  /** Forwarded to the underlying PipelineDriver (ReadFrom page size). Default 500. */
  limit?: number
  /** Defaults to a fresh InProcessTransport. */
  transport?: WorkerTransport
}

/** One processed page's outcome, kept for the debug panel's "last pump" line. */
export interface RuntimeSummary {
  batchSize: number
  outputCount: number
  discardCount: number
  errorCount: number
  completedAt: number
}

export function usePipelineRuntime(client: PipelineClient, flow: Flow, options: PipelineRuntimeOptions = {}) {
  let lastBatchSize = 0
  const observingClient: PipelineClient = {
    async readFrom(consumer, limit) {
      const batch = await client.readFrom(consumer, limit)
      lastBatchSize = (batch ?? []).length
      return batch
    },
    commit: (batch) => client.commit(batch),
  }

  const driver = new PipelineDriver(observingClient, flow, options)
  const running = ref(false)
  const pumping = ref(false)
  const lastRun = ref<RuntimeSummary | null>(null)
  const error = ref<string | null>(null)

  // A pump request arriving while a page is in flight sets this latch rather
  // than starting a concurrent read/commit cycle. The drain checks it after
  // every read, closing the empty-read/event-arrival race.
  let wakePending = false
  let generation = 0
  let drainPromise: Promise<void> | null = null

  async function drain(token: number): Promise<void> {
    while (running.value && token === generation) {
      wakePending = false
      let result
      try {
        result = await driver.pump(() => running.value && token === generation)
      } catch (err) {
        if (running.value && token === generation) {
          console.warn('pipeline runtime: pump failed', err)
          error.value = err instanceof Error && err.message ? err.message : 'Pump failed'
        }
        return
      }

      // stop() (used during a flow switch) prevents a returned page from
      // running or committing, and prevents the old runtime from publishing
      // stale state or starting another page.
      if (!running.value || token !== generation) return

      // Preserve the latest useful page summary while draining: the required
      // terminating empty read should not erase the final processed page.
      if (result || lastRun.value === null) {
        const errorCount = result ? result.nodeRuns.filter((r) => !r.ok).length : 0
        lastRun.value = {
          batchSize: lastBatchSize,
          outputCount: result?.outputs.length ?? 0,
          discardCount: result?.discards.length ?? 0,
          errorCount,
          completedAt: Date.now(),
        }
      }
      error.value = null

      // A non-empty page means there may be another one. An in-flight wakeup
      // means retry even after an empty page, so notification delivery is
      // level-triggered instead of edge-triggered.
      if (result === null && !wakePending) return
      await new Promise<void>((resolve) => setTimeout(resolve, 0))
    }
  }

  /**
   * Requests a coalesced drain. Concurrent callers share the active drain;
   * their wakeup is latched so no appended work is lost in flight.
   */
  function pump(): Promise<void> {
    if (!running.value) return Promise.resolve()
    wakePending = true
    if (drainPromise) return drainPromise

    const token = generation
    pumping.value = true
    drainPromise = drain(token).finally(() => {
      if (token === generation) pumping.value = false
      drainPromise = null
    })
    return drainPromise
  }

  /** Starts the runtime and immediately drains any persisted backlog. */
  async function run(): Promise<void> {
    if (running.value) return
    running.value = true
    await pump()
  }

  /** Stops this drain generation; an in-flight page may finish, but no next page starts. */
  function stop(): void {
    running.value = false
    generation++
    wakePending = false
  }

  return {
    running,
    pumping,
    lastRun,
    error,
    offset: computed(() => driver.offset),
    run,
    stop,
    pump,
  }
}
