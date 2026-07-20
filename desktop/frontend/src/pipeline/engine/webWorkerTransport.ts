// Production WorkerTransport: one shared worker hosts every isolate:false
// runtime (github-filter today); a dedicated worker is spawned per
// isolate:true node INSTANCE (the function node), so a timeout can kill and
// replace only that function instance's worker.

import type { Msg } from '../types'
import { NodeTimeoutError, type NodeResult, type WorkerTransport } from './transport'

/** The minimal Worker surface this transport needs — real `Worker` satisfies it, and tests inject a fake. */
export interface WorkerLike {
  postMessage(data: unknown): void
  terminate(): void
  onmessage: ((ev: any) => void) | null
  onerror: ((ev: any) => void) | null
}

/**
 * Builds the actual worker for a hosting slot: 'shared' for every
 * isolate:false runtime type (one worker for all of them, keyed 'shared');
 * 'isolated' for one instanceId's isolate:true node. workerFactory.ts
 * provides the Vite module-worker implementation; tests inject fakes.
 */
export type WorkerFactory = (kind: 'shared' | 'isolated', instanceId: string) => WorkerLike

interface RunRequest {
  kind: 'run'
  id: number
  runtimeType: string
  instanceId: string
  config: unknown
  msg: Msg
  state: Record<string, any>
}

interface RunResponse {
  id: number
  result?: NodeResult
  /** The worker's own (structured-clone) copy of state after running, merged back onto the caller's object — see the class doc. */
  state?: Record<string, any>
  error?: string
}

interface PendingRequest {
  resolve(v: NodeResult): void
  reject(e: unknown): void
  /** The worker servicing this request, used to scope worker-level errors. */
  worker: WorkerLike
  /** The caller's original state object (NOT the cloned copy the worker received) — mutations reported back in the response are merged onto this. */
  state: Record<string, any>
}

/**
 * WebWorkerTransport owns request/response correlation, timeout-driven
 * worker replacement, and merging returned state across the postMessage
 * structured-clone boundary. workerFactory.ts supplies the production Vite
 * module-worker factory; tests inject a fake WorkerLike to exercise this
 * protocol without a browser worker implementation.
 *
 * State across the postMessage boundary: `state` is structured-cloned to
 * the worker on every call, so the worker mutates its OWN copy, not the
 * caller's object directly (unlike InProcessTransport's same-thread
 * sharing). The worker is expected to send its mutated copy back in the
 * response, which this transport merges (Object.assign) onto the *original*
 * state object passed into run() — so a caller persisting state across
 * ticks (e.g. runGraph's `states` map) observes the same mutations it would
 * under InProcessTransport.
 */
export class WebWorkerTransport implements WorkerTransport {
  private sharedWorker: WorkerLike | null = null
  private isolatedWorkers = new Map<string, WorkerLike>()
  private pending = new Map<number, PendingRequest>()
  // runGraph calls reset() after any NodeTimeoutError. A timeout here has
  // already installed a clean replacement, so consume that follow-up reset
  // instead of terminating the new worker.
  private replacedAfterTimeout = new Set<string>()
  private nextId = 1
  private disposed = false

  constructor(
    private readonly createWorker: WorkerFactory,
    private readonly isolateTypes: Set<string> = new Set(['function']),
  ) {}

  async run(runtimeType: string, instanceId: string, config: unknown, msg: Msg, state: object, timeoutMs: number): Promise<NodeResult> {
    if (this.disposed) throw new Error('pipeline worker transport has been disposed')

    const isolated = this.isolateTypes.has(runtimeType)
    const worker = isolated ? this.getIsolatedWorker(instanceId) : this.getSharedWorker()

    const id = this.nextId++
    const originalState = state as Record<string, any>
    const request: RunRequest = { kind: 'run', id, runtimeType, instanceId, config, msg, state: originalState }

    const responsePromise = new Promise<NodeResult>((resolve, reject) => {
      this.pending.set(id, { resolve, reject, worker, state: originalState })
    })

    worker.postMessage(request)

    try {
      return await this.raceTimeout(responsePromise, timeoutMs, id, worker, isolated, instanceId)
    } finally {
      this.pending.delete(id)
    }
  }

  reset(instanceId: string): void {
    if (this.disposed || this.replacedAfterTimeout.delete(instanceId)) return
    const worker = this.isolatedWorkers.get(instanceId)
    if (!worker) return
    worker.terminate()
    this.isolatedWorkers.delete(instanceId)
  }

  /**
   * Permanently releases every worker this transport owns. Unlike reset(),
   * this also terminates the shared worker and rejects all outstanding work.
   */
  dispose(): void {
    if (this.disposed) return
    this.disposed = true

    const workers = new Set<WorkerLike>(this.isolatedWorkers.values())
    if (this.sharedWorker) workers.add(this.sharedWorker)
    for (const worker of workers) {
      worker.onmessage = null
      worker.onerror = null
      worker.terminate()
    }
    this.sharedWorker = null
    this.isolatedWorkers.clear()
    this.replacedAfterTimeout.clear()

    for (const waiter of this.pending.values()) {
      waiter.reject(new Error('pipeline worker transport has been disposed'))
    }
    this.pending.clear()
  }

  private raceTimeout(promise: Promise<NodeResult>, timeoutMs: number, id: number, worker: WorkerLike, isolated: boolean, instanceId: string): Promise<NodeResult> {
    return new Promise<NodeResult>((resolve, reject) => {
      const timer = setTimeout(() => {
        const waiter = this.pending.get(id)
        if (!waiter) return
        this.pending.delete(id)
        this.replaceWorker(worker, isolated, instanceId, id)
        const error = new NodeTimeoutError(timeoutMs)
        waiter.reject(error)
        reject(error)
      }, timeoutMs)
      promise.then(
        (v) => {
          clearTimeout(timer)
          resolve(v)
        },
        (e) => {
          clearTimeout(timer)
          reject(e)
        },
      )
    })
  }

  private getSharedWorker(): WorkerLike {
    if (!this.sharedWorker) {
      this.sharedWorker = this.createWorker('shared', 'shared')
      this.wire(this.sharedWorker)
    }
    return this.sharedWorker
  }

  private getIsolatedWorker(instanceId: string): WorkerLike {
    let worker = this.isolatedWorkers.get(instanceId)
    if (!worker) {
      worker = this.createWorker('isolated', instanceId)
      this.wire(worker)
      this.isolatedWorkers.set(instanceId, worker)
    }
    return worker
  }

  /** Terminates a timed-out worker and immediately installs a clean replacement. */
  private replaceWorker(worker: WorkerLike, isolated: boolean, instanceId: string, timedOutID: number): void {
    worker.terminate()

    if (isolated) {
      if (this.isolatedWorkers.get(instanceId) === worker) {
        const replacement = this.createWorker('isolated', instanceId)
        this.wire(replacement)
        this.isolatedWorkers.set(instanceId, replacement)
      }
    } else if (this.sharedWorker === worker) {
      const replacement = this.createWorker('shared', 'shared')
      this.wire(replacement)
      this.sharedWorker = replacement
    }

    this.replacedAfterTimeout.add(instanceId)
    for (const [id, pending] of this.pending) {
      if (id === timedOutID || pending.worker !== worker) continue
      this.pending.delete(id)
      pending.reject(new Error('pipeline worker terminated after another request timed out'))
    }
  }

  private wire(worker: WorkerLike): void {
    worker.onmessage = (ev) => {
      const response = ev.data as RunResponse
      const waiter = this.pending.get(response.id)
      if (!waiter) return // late/duplicate response (e.g. arrived after a timeout already settled this id) — ignore
      this.pending.delete(response.id)
      if (response.error) {
        waiter.reject(new Error(response.error))
        return
      }
      if (response.state) Object.assign(waiter.state, response.state)
      waiter.resolve(response.result ?? null)
    }
    worker.onerror = (ev) => {
      // A worker-level error carries no request id to correlate — reject
      // every still-pending request against this worker rather than
      // leaving them hanging forever.
      const message = ev && typeof ev === 'object' && typeof (ev as any).message === 'string' ? (ev as any).message : String(ev)
      for (const [id, waiter] of this.pending) {
        if (waiter.worker !== worker) continue
        waiter.reject(new Error(message))
        this.pending.delete(id)
      }
    }
  }
}
