// The cursor driver: a pure, standalone loop over one Flow's read cursor.
// Not mounted anywhere yet (Phase 6/7's composable owns subscribing to the
// "log:appended" Wails event and calling pump() — see
// composables/useFeedState.ts's Events.On pattern for what that will look
// like) — this module is unit-tested on its own.

import type { CommitResult, Flow, Msg } from './types'
import { InProcessTransport, type WorkerTransport } from './engine/transport'
import { runGraph, type RunGraphOptions } from './engine/runGraph'
import { processorRegistry } from './registry'

/**
 * The subset of PipelineService the driver needs. Injected rather than
 * imported directly from bindings/ or @wailsio/runtime so the driver is
 * testable in isolation — a thin adapter over the generated
 * ReadFrom/Commit bindings satisfies this shape (CommitResult is
 * structurally assignable to the generated CommitBatch type, see types.ts).
 */
export interface PipelineClient {
  readFrom(offset: number, limit: number): Promise<Msg[] | null | undefined>
  commit(batch: CommitResult): Promise<void>
}

export interface PipelineDriverOptions {
  /** ReadFrom page size per pump(). Default 500. */
  limit?: number
  /** Cursor to start from (e.g. a previously-committed offset persisted elsewhere). Default 0. */
  fromOffset?: number
  /** Defaults to a fresh InProcessTransport over the built-in worker registry (registry.ts) — the documented main-thread fallback; production wiring passes a WebWorkerTransport instead. */
  transport?: WorkerTransport
  /** Forwarded to every runGraph call. `states` defaults to one Map owned by this driver instance (see below) unless overridden. */
  runGraph?: RunGraphOptions
}

/**
 * PipelineDriver holds a read cursor over one Flow: pump() reads the next
 * page from the log via ReadFrom, runs the graph, Commits the result, and
 * advances the cursor to the committed upToOffset.
 *
 * Idempotent by construction: pump() derives everything from
 * `client.readFrom(cursor, limit)`, which is a pure read — calling pump()
 * again without the log having grown re-reads the same messages and
 * re-runs the same (deterministic) graph computation, producing a
 * byte-identical CommitResult. Go's CommitBatch is itself idempotent on
 * offset (a replayed upToOffset <= the consumer's stored offset is a
 * no-op), so a duplicate commit — whether from a redundant pump() or a
 * stale retry — is always safe.
 */
export class PipelineDriver {
  private cursor: number
  private readonly transport: WorkerTransport
  private readonly runGraphOptions: RunGraphOptions
  private readonly limit: number

  constructor(
    private readonly client: PipelineClient,
    private readonly flow: Flow,
    opts: PipelineDriverOptions = {},
  ) {
    this.cursor = opts.fromOffset ?? 0
    this.limit = opts.limit ?? 500
    this.transport = opts.transport ?? new InProcessTransport(processorRegistry)
    // A function node's state must survive across pumps, not just within
    // one runGraph call — this driver owns one persistent Map for its
    // whole lifetime unless the caller supplies its own.
    this.runGraphOptions = { states: new Map(), ...opts.runGraph }
  }

  /** The last offset committed, or the starting offset if pump() hasn't run yet. */
  get offset(): number {
    return this.cursor
  }

  /**
   * Reads, runs, and commits one page. Returns null (a no-op — the cursor
   * is unchanged) when there was nothing new to read.
   */
  async pump(): Promise<CommitResult | null> {
    const batch = (await this.client.readFrom(this.cursor, this.limit)) ?? []
    if (batch.length === 0) return null

    const result = await runGraph(this.flow, batch, this.transport, this.runGraphOptions)
    await this.client.commit(result)
    this.cursor = result.upToOffset
    return result
  }
}
