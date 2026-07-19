// Mounts a PipelineDriver for one engine Flow and drives its pump() loop
// end-to-end (source log -> graph -> commit) inside the running app. This is
// Phase 6c's runtime hookup — the first place anything actually calls
// driver.ts's pump() outside a test.
//
// Mirrors driver.ts's and usePipelineEditor.ts's injection posture: this
// composable's core takes an injected PipelineClient (driver.ts's own
// interface — reused here rather than redeclared, since the shape is
// identical) and never imports bindings/ or @wailsio/runtime directly. A
// fresh InProcessTransport is wired in by default unless the caller
// supplies one — see driver.ts's PipelineDriverOptions.transport docs for
// why InProcessTransport (not WorkerTransport) is the right default for v1:
// it's the Web Worker Wails spike's documented main-thread fallback, needs
// no worker-bundle wiring, and satisfies the exact same WorkerTransport
// contract runGraph expects.
//
// The "log:appended" Wails event subscription is deliberately NOT owned
// here — same reasoning driver.ts's own module docs give: the mounting
// component owns Events.On/Off and calls the pump() this composable
// exposes on every tick, exactly like usePipelineEditor's "flows:updated"
// subscription is owned by FlowsView.vue rather than living in the
// composable core. That keeps this composable importable and unit-testable
// with zero @wailsio/runtime dependency.
import { computed, ref } from 'vue'
import { PipelineDriver, type PipelineClient } from '../driver'
import type { Flow } from '../types'
import type { WorkerTransport } from '../engine/transport'

export interface PipelineRuntimeOptions {
  /** Forwarded to the underlying PipelineDriver (ReadFrom page size). Default 500. */
  limit?: number
  /** Forwarded to the underlying PipelineDriver (starting cursor). Default 0. */
  fromOffset?: number
  /** Defaults to a fresh InProcessTransport — see driver.ts's PipelineDriverOptions.transport docs. */
  transport?: WorkerTransport
}

/** One pump()'s outcome, kept for the debug panel's "last pump" line. */
export interface RuntimeSummary {
  /** Msgs read off the log this pump (0 for a clean no-op — nothing new since the last pump). */
  batchSize: number
  outputCount: number
  discardCount: number
  /** Count of this pump's node_run rows with ok:false (a node errored or timed out). */
  errorCount: number
  /** Date.now() when this pump finished. */
  completedAt: number
}

export function usePipelineRuntime(client: PipelineClient, flow: Flow, options: PipelineRuntimeOptions = {}) {
  // PipelineDriver.pump()'s return value (CommitResult) only ever describes
  // what came OUT of a pump (outputs/discards/nodeRuns) — it never carries
  // the input batch or its size. This thin wrapper around the injected
  // client is a pass-through in every respect except it also stashes the
  // batch length just before handing it back to the driver, purely so
  // RuntimeSummary can report "N msgs in" without the driver itself needing
  // to change shape.
  let lastBatchSize = 0
  const observingClient: PipelineClient = {
    async readFrom(offset, limit) {
      const batch = await client.readFrom(offset, limit)
      lastBatchSize = (batch ?? []).length
      return batch
    },
    commit: (batch) => client.commit(batch),
  }

  // The driver owns the flow's commit consumer (flow.id) and cursor for
  // this composable's whole lifetime.
  const driver = new PipelineDriver(observingClient, flow, options)

  const running = ref(false)
  // Guards against overlapping pumps: a "log:appended" wake-up firing while
  // a previous pump is still in flight (e.g. a slow node run) must not
  // start a second concurrent readFrom/commit cycle over the same cursor.
  const pumping = ref(false)
  const lastRun = ref<RuntimeSummary | null>(null)
  const error = ref<string | null>(null)

  /**
   * Reads, runs, and commits one page via the underlying PipelineDriver. A
   * no-op when not running — run()/stop() gate whether an external
   * "log:appended" wake-up (or this composable's own initial pump) is
   * allowed to advance the cursor — or when a pump is already in flight.
   */
  async function pump(): Promise<void> {
    if (!running.value || pumping.value) return
    pumping.value = true
    try {
      const result = await driver.pump()
      const errorCount = result ? result.nodeRuns.filter((r) => !r.ok).length : 0
      lastRun.value = {
        batchSize: lastBatchSize,
        outputCount: result?.outputs.length ?? 0,
        discardCount: result?.discards.length ?? 0,
        errorCount,
        completedAt: Date.now(),
      }
      error.value = null
    } catch (err) {
      // A pump failure (e.g. Commit rejecting) is surfaced, not swallowed —
      // the cursor stays put (PipelineDriver only advances it after a
      // successful commit), so the next pump retries the same page.
      console.warn('pipeline runtime: pump failed', err)
      error.value = err instanceof Error && err.message ? err.message : 'Pump failed'
    } finally {
      pumping.value = false
    }
  }

  /** Starts the runtime and performs an immediate initial pump (proves the loop end-to-end without waiting for the next "log:appended"). Idempotent while already running. */
  async function run(): Promise<void> {
    if (running.value) return
    running.value = true
    await pump()
  }

  /** Stops the runtime — a subsequent "log:appended" wake-up calling pump() is a no-op until run() again. Does not reset the cursor or lastRun. */
  function stop(): void {
    running.value = false
  }

  return {
    running,
    pumping,
    lastRun,
    error,
    /** The driver's committed offset, exposed for debug display. */
    offset: computed(() => driver.offset),
    run,
    stop,
    pump,
  }
}
