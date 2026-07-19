// Production WorkerTransport: one shared worker hosts every isolate:false
// runtime (github-filter today); a dedicated worker is spawned per
// isolate:true node INSTANCE (the function node), so a timeout's
// terminate() kills only that one instance. See the class doc below for
// why this file only owns the message-protocol glue, not a real worker
// bundle.

import type { Msg } from '../types'
import { NodeTimeoutError, type NodeResult, type WorkerTransport } from './transport'

/** The minimal Worker surface this transport needs — real `Worker` satisfies it, and tests inject a fake. */
export interface WorkerLike {
  postMessage(data: unknown): void
  terminate(): void
  onmessage: ((ev: { data: any }) => void) | null
  onerror: ((ev: any) => void) | null
}

/**
 * Builds the actual worker for a hosting slot: 'shared' for every
 * isolate:false runtime type (one worker for all of them, keyed
 * 'shared'); 'isolated' for one instanceId's isolate:true node. Production
 * wiring (spawning a real `new Worker(new URL('./workerEntry.ts',
 * import.meta.url), {type: 'module'})` bundle hosting the worker registry)
 * is the caller's responsibility — there is no default factory here.
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
  /** The caller's original state object (NOT the cloned copy the worker received) — mutations reported back in the response are merged onto this. */
  state: Record<string, any>
}

/**
 * WebWorkerTransport owns only the message-protocol glue (request/response
 * correlation by id, timeout -> terminate -> respawn, merging a worker's
 * returned state back onto the caller's object across the postMessage
 * structured-clone boundary) — it does not bundle or load an actual worker
 * script itself. `createWorker` is required (no default): wiring it to a
 * real worker bundle is production glue with nothing to unit-test here (per
 * the Web Worker Wails spike, real execution is verified by manual QA, not
 * vitest/happy-dom, which cannot construct real module workers). Tests
 * inject a fake WorkerLike to exercise the protocol logic in this file.
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
  private nextId = 1

  constructor(
    private readonly createWorker: WorkerFactory,
    private readonly isolateTypes: Set<string> = new Set(['function']),
  ) {}

  async run(runtimeType: string, instanceId: string, config: unknown, msg: Msg, state: object, timeoutMs: number): Promise<NodeResult> {
    const isolated = this.isolateTypes.has(runtimeType)
    const worker = isolated ? this.getIsolatedWorker(instanceId) : this.getSharedWorker()

    const id = this.nextId++
    const originalState = state as Record<string, any>
    const request: RunRequest = { kind: 'run', id, runtimeType, instanceId, config, msg, state: originalState }

    const responsePromise = new Promise<NodeResult>((resolve, reject) => {
      this.pending.set(id, { resolve, reject, state: originalState })
    })

    worker.postMessage(request)

    try {
      return await this.raceTimeout(responsePromise, timeoutMs, id, isolated, instanceId)
    } finally {
      this.pending.delete(id)
    }
  }

  reset(instanceId: string): void {
    const worker = this.isolatedWorkers.get(instanceId)
    if (worker) {
      worker.terminate()
      this.isolatedWorkers.delete(instanceId)
    }
    // The shared worker hosts every isolate:false instance at once —
    // terminating it over one instance's timeout would kill unrelated
    // in-flight work, so a shared-hosted instance's reset is a no-op (no
    // isolate:false runtime today — github-filter — does anything async
    // enough to realistically time out).
  }

  private raceTimeout(promise: Promise<NodeResult>, timeoutMs: number, id: number, isolated: boolean, instanceId: string): Promise<NodeResult> {
    return new Promise<NodeResult>((resolve, reject) => {
      const timer = setTimeout(() => {
        if (isolated) {
          this.isolatedWorkers.get(instanceId)?.terminate()
          this.isolatedWorkers.delete(instanceId)
        }
        this.pending.delete(id)
        reject(new NodeTimeoutError(timeoutMs))
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
        waiter.reject(new Error(message))
        this.pending.delete(id)
      }
    }
  }
}
