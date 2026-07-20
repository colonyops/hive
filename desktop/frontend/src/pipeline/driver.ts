// The consumer-driven graph runner for one Flow. The backend owns the
// durable consumer cursor, so a new driver resumes from SQLite rather than
// replaying the log from a frontend-owned zero cursor.

import type { CommitResult, Flow, Msg } from './types'
import { InProcessTransport, type WorkerTransport } from './engine/transport'
import { runGraph, type RunGraphOptions } from './engine/runGraph'
import { processorRegistry } from './registry'

/** The subset of PipelineService the driver needs. */
export interface PipelineClient {
  /** Reads the next page after this consumer's persisted SQLite offset. */
  readFrom(consumer: string, limit: number): Promise<Msg[] | null | undefined>
  commit(batch: CommitResult): Promise<void>
}

export interface PipelineDriverOptions {
  /** ReadFrom page size per pump(). Default 500. */
  limit?: number
  /** Defaults to a fresh InProcessTransport over the built-in worker registry. */
  transport?: WorkerTransport
  /** Forwarded to every runGraph call. */
  runGraph?: RunGraphOptions
}

/**
 * PipelineDriver processes one page for a Flow's consumer. ReadFrom resolves
 * the starting position from SQLite on every page; the driver only remembers
 * the most recently committed decimal offset for debug display.
 */
export class PipelineDriver {
  private lastCommittedOffset: string | null = null
  private readonly transport: WorkerTransport
  private readonly runGraphOptions: RunGraphOptions
  private readonly limit: number

  constructor(
    private readonly client: PipelineClient,
    private readonly flow: Flow,
    opts: PipelineDriverOptions = {},
  ) {
    this.limit = opts.limit ?? 500
    this.transport = opts.transport ?? new InProcessTransport(processorRegistry)
    // A function node's state must survive across pages, not just within one
    // runGraph call.
    this.runGraphOptions = { states: new Map(), ...opts.runGraph }
  }

  /** The latest decimal offset committed by this driver, if it has committed. */
  get offset(): string | null {
    return this.lastCommittedOffset
  }

  /**
   * Reads, runs, and commits one page. The backend derives the read position
   * from flow.id's persisted consumer checkpoint, so this is safe after a
   * frontend restart and never relies on a local cursor.
   */
  async pump(shouldContinue: () => boolean = () => true): Promise<CommitResult | null> {
    const batch = (await this.client.readFrom(this.flow.id, this.limit)) ?? []
    if (!shouldContinue() || batch.length === 0) return null

    const result = await runGraph(this.flow, batch, this.transport, this.runGraphOptions)
    if (!shouldContinue()) return null
    await this.client.commit(result)
    this.lastCommittedOffset = result.upToOffset
    return result
  }
}
