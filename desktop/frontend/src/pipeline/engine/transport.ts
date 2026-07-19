// The transport seam: runGraph never touches a real Web Worker directly. It
// drives every processor node through a WorkerTransport, so tests (and the
// documented main-thread fallback from the Web Worker Wails spike,
// .hive/research/2026-07-18-web-worker-wails-spike.md) can run entirely
// in-process — happy-dom/vitest cannot construct real module workers with
// terminate(), so InProcessTransport is both the test double and the
// shipped fallback, not a mock-only shim.

import type { Msg } from '../types'

/**
 * NodeResult is what a processor's onMsg (or a function node's compiled
 * on_message) returns for one input msg:
 *   - a single Msg                       -> port 0
 *   - Msg[]                              -> multiple messages, all port 0
 *   - a port-indexed array (msg|msg[]|null)[] -> array[i] is what goes out
 *     port i (an array element itself an array = multiple msgs on that
 *     port; null = nothing on that port this call)
 *   - null                                -> discard (no output at all)
 *
 * Msg[] and the port-indexed array shape overlap syntactically (both are
 * plain arrays) — the engine disambiguates using the node's declared output
 * count (from that node type's own config.ts), not the array's contents.
 * See engine/runGraph.ts's normalizeResult.
 */
export type NodeResult = Msg | Array<Msg | Msg[] | null> | Msg[] | null

/**
 * NodeContext is what a ProcessorRuntime's lifecycle hooks receive: the
 * node's own (per-type) config, and a per-instance state object that
 * survives across messages for the lifetime of one running flow (not
 * durable — a restart forgets it). The engine (or the transport hosting a
 * worker) owns the identity of this object; runtimes only ever read/mutate
 * it.
 */
export interface NodeContext<C = Record<string, any>> {
  config: C
  state: Record<string, any>
}

/**
 * ProcessorRuntime is the worker-side contract a node type's runtime.ts
 * implements (D2). start/stop are optional lifecycle hooks (only the
 * `function` node uses them, for on_start/on_stop); onMsg is required.
 */
export interface ProcessorRuntime<C = Record<string, any>> {
  type: string
  start?(ctx: NodeContext<C>): void | Promise<void>
  onMsg(msg: Msg, ctx: NodeContext<C>): NodeResult | Promise<NodeResult>
  stop?(ctx: NodeContext<C>): void | Promise<void>
}

/**
 * Thrown (by a transport, or observed by runGraph) when a node's run did not
 * complete within timeoutMs. Distinguished from an ordinary thrown error so
 * runGraph knows to call transport.reset(instanceId) — the "terminate,
 * respawn" step the design gives only to timeouts, not to ordinary node
 * errors (a node that threw and returned control needs no reset).
 */
export class NodeTimeoutError extends Error {
  constructor(public readonly timeoutMs: number) {
    super(`node run did not complete within ${timeoutMs}ms`)
    this.name = 'NodeTimeoutError'
  }
}

/**
 * WorkerTransport is the engine's sole seam onto "run this node for this
 * message" — it never hard-depends on a real Web Worker. `run` executes one
 * message through one node instance; `reset` is called by the engine after
 * a timeout to terminate/respawn whatever backs instanceId, so the next run
 * starts clean.
 */
export interface WorkerTransport {
  run(runtimeType: string, instanceId: string, config: unknown, msg: Msg, state: object, timeoutMs: number): Promise<NodeResult>
  reset(instanceId: string): void
}

/** Races a promise against a deadline, rejecting with NodeTimeoutError if the deadline wins. */
export function raceTimeout<T>(promise: Promise<T>, timeoutMs: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new NodeTimeoutError(timeoutMs)), timeoutMs)
    promise.then(
      (value) => {
        clearTimeout(timer)
        resolve(value)
      },
      (error) => {
        clearTimeout(timer)
        reject(error)
      },
    )
  })
}

/**
 * InProcessTransport runs a ProcessorRuntime directly in the calling
 * thread, wrapped in a Promise. This is both the unit-test double (tests
 * inject a registry with a "slow" runtime to exercise the timeout path) and
 * the documented main-thread fallback if Web Workers ever turn out to be
 * unavailable on a target webview (they are expected to be available on all
 * three Wails v3 targets — see the spike — so this is a fallback, not the
 * primary production path).
 *
 * Enforces timeoutMs itself (cooperatively: racing the returned promise
 * against a deadline) so it is correct standalone, not only when driven
 * through runGraph, which applies the same "record error + discard + reset
 * on timeout" handling uniformly across transports. A truly blocking
 * synchronous loop in a node's code cannot be preempted this way — only a
 * real Worker's terminate() can do that — but a node that returns a slow
 * Promise (the realistic "stuck" case for in-process/main-thread execution)
 * is caught correctly.
 */
export class InProcessTransport implements WorkerTransport {
  private started = new Set<string>()

  constructor(private readonly registry: Record<string, ProcessorRuntime>) {}

  async run(runtimeType: string, instanceId: string, config: unknown, msg: Msg, state: object, timeoutMs: number): Promise<NodeResult> {
    const runtime = this.registry[runtimeType]
    if (!runtime) throw new Error(`InProcessTransport: no processor runtime registered for type "${runtimeType}"`)

    const ctx: NodeContext = { config: config as Record<string, any>, state: state as Record<string, any> }

    const exec = async (): Promise<NodeResult> => {
      if (!this.started.has(instanceId)) {
        this.started.add(instanceId)
        await runtime.start?.(ctx)
      }
      return await runtime.onMsg(msg, ctx)
    }

    return await raceTimeout(exec(), timeoutMs)
  }

  reset(instanceId: string): void {
    // "Respawn": the next run() for this instance re-invokes start().
    this.started.delete(instanceId)
  }
}
