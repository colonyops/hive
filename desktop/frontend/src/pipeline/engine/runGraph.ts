// The graph engine: executes one Flow over one batch of Msgs (from
// PipelineService.ReadFrom) in topological order, producing a CommitResult
// that accounts for every input message — as a tagged terminal output, a
// discard, or (on a node error/timeout) an errored discard. See the module
// docs on WorkerTransport (./transport.ts) for why node execution never
// touches a real Worker directly here.

import type { CommitResult, Discard, FeedSnapshot, Flow, FlowNode, Msg, Output, Sink, Wire } from '../types'
import * as actionNode from '../nodes/action/config'
import * as functionNode from '../nodes/function/config'
import * as githubFilterNode from '../nodes/github-filter/config'
import * as githubSourceNode from '../nodes/github-source/config'
import * as feedNode from '../nodes/feed/config'
import { inDegrees, outWiresByPort, topoSort } from './graph'
import { NodeTimeoutError, type NodeResult, type WorkerTransport } from './transport'

export interface RunGraphOptions {
  /**
   * Per-node-instance state (instanceId = `${flow.id}:${node.id}`),
   * persisted by the caller across multiple runGraph calls so a function
   * node's state survives across pump ticks (not durable — a restart
   * forgets it, matching the design's "stateless frontend" posture: only
   * this in-memory object, never anything Go persists). runGraph creates a
   * fresh empty map when omitted, which is fine for a single call/most
   * tests; a driver that pumps repeatedly should hold one Map and pass the
   * same instance every call.
   */
  states?: Map<string, Record<string, any>>
  /**
   * Fallback per-node timeout (ms) when a node's own config has no
   * `timeout` field. Default matches the function node's D1 default
   * (5000ms).
   */
  defaultTimeoutMs?: number
}

/** Discard.nodeId used when an input msg matched no entry node in the flow (e.g. its Topic belongs to a different flow's source) — still accounted for so the offset can advance past it. */
export const UNROUTED_NODE_ID = '$unrouted'

// Node types the engine forwards without calling a transport at all: source
// nodes run on the backend (D1) — the frontend only relays whatever was
// routed to them on to their wires.
const PASSTHROUGH_TYPES = new Set<string>([githubSourceNode.type])

interface TerminalDef {
  sink(flowId: string, nodeId: string, config: any): Sink
  unread: boolean
}

// The two terminal node types' sink-tagging is delegated to each node's own
// config.ts (single source of truth for its sink shape) rather than
// re-encoded here.
const TERMINALS: Record<string, TerminalDef> = {
  [feedNode.type]: { sink: feedNode.sink, unread: feedNode.unread },
  [actionNode.type]: { sink: actionNode.sink, unread: actionNode.unread },
}

interface NodeRunAcc {
  inCount: number
  outCount: number
  dropCount: number
  ok: boolean
  err: string
  durMs: number
}

interface SnapshotContext {
  sourceTopic: string
  snapshotId: string
}

type RuntimeMsg = Msg & { snapshotContext?: SnapshotContext }

export async function runGraph(flow: Flow, batch: Msg[], transport: WorkerTransport, opts: RunGraphOptions = {}): Promise<CommitResult> {
  const nodesById = new Map(flow.nodes.map((n) => [n.id, n]))
  const order = topoSort(flow)
  const outWires = outWiresByPort(flow)
  const entryDegrees = inDegrees(flow)
  const states = opts.states ?? new Map<string, Record<string, any>>()
  const defaultTimeoutMs = opts.defaultTimeoutMs ?? functionNode.DEFAULT_TIMEOUT_MS

  const pending = new Map<string, RuntimeMsg[]>()
  const snapshots = new Map<string, FeedSnapshot>()
  const outputs: Output[] = []
  const discards: Discard[] = []
  const nodeRuns = new Map<string, NodeRunAcc>()

  // A source snapshot contains every current item, even items whose payload
  // did not change. Route those items normally, but tag their resulting feed
  // outputs so CommitBatch can atomically remove the source's obsolete rows.
  const entryNodeIds = flow.nodes.filter((n) => (entryDegrees.get(n.id) ?? 0) === 0).map((n) => n.id)
  for (const msg of batch) {
    const matchingEntries = entryNodeIds.filter((id) => acceptsEntry(flow.id, nodesById.get(id)!, msg))
    if (matchingEntries.length === 0) {
      discards.push({ msgId: msg.ID, nodeId: UNROUTED_NODE_ID })
      continue
    }

    if (msg.Snapshot != null) {
      const context = { sourceTopic: msg.Topic, snapshotId: msg.ID }
      for (const node of flow.nodes) {
        if (node.type !== feedNode.type) continue
        const feedID = feedNode.sink(flow.id, node.id).targetId
        snapshots.set(`${feedID}\u0000${context.sourceTopic}`, { feedId: feedID, sourceTopic: context.sourceTopic, snapshotId: context.snapshotId })
      }
      for (const item of msg.Snapshot) {
        const itemMsg: RuntimeMsg = { ...msg, Key: item.key, Payload: item.payload, Snapshot: null, snapshotContext: context }
        matchingEntries.forEach((id) => pushPending(pending, id, matchingEntries.length > 1 ? structuredClone(itemMsg) : itemMsg))
      }
      continue
    }

    matchingEntries.forEach((id) => pushPending(pending, id, matchingEntries.length > 1 ? structuredClone(msg) : msg))
  }

  for (const nodeId of order) {
    const msgs = pending.get(nodeId)
    if (!msgs || msgs.length === 0) continue
    const node = nodesById.get(nodeId)!
    const run = acc(nodeRuns, nodeId)

    for (const msg of msgs) {
      run.inCount++

      if (node.disabled) {
        run.dropCount++
        discards.push({ msgId: msg.ID, nodeId })
        continue
      }

      if (PASSTHROUGH_TYPES.has(node.type)) {
        forward(pending, outWires, nodeId, 0, msg, run, discards)
        continue
      }

      const terminal = TERMINALS[node.type]
      if (terminal) {
        const sink = terminal.sink(flow.id, nodeId, node.config)
        const output: Output = { sink, key: msg.Key, payload: msg.Payload, unread: terminal.unread }
        if (msg.snapshotContext && sink.kind === 'feed') {
          output.sourceTopic = msg.snapshotContext.sourceTopic
          output.snapshotId = msg.snapshotContext.snapshotId
          output.preserveUnread = true
        }
        outputs.push(output)
        run.outCount++
        continue
      }

      const instanceId = `${flow.id}:${nodeId}`
      const timeoutMs = typeof node.config.timeout === 'number' ? node.config.timeout : defaultTimeoutMs
      const state = states.get(instanceId) ?? seedState(states, instanceId)

      const startedAt = nowMs()
      try {
        const result = await transport.run(node.type, instanceId, node.config, msg, state, timeoutMs)
        run.durMs += nowMs() - startedAt

        const produced = normalizeResult(result, outputCount(node))
        if (produced.length === 0) {
          run.dropCount++
          discards.push({ msgId: msg.ID, nodeId })
          continue
        }
        for (const { port, msg: outMsg } of produced) {
          forward(pending, outWires, nodeId, port, msg.snapshotContext ? { ...outMsg, snapshotContext: msg.snapshotContext } : outMsg, run, discards)
        }
      } catch (error) {
        run.durMs += nowMs() - startedAt
        run.ok = false
        run.err = error instanceof Error ? error.message : String(error)
        run.dropCount++
        discards.push({ msgId: msg.ID, nodeId })
        // Timeout, specifically, means the node instance may be stuck —
        // "terminate, respawn" per the design. An ordinary thrown error
        // means the node returned control fine and needs no reset.
        if (error instanceof NodeTimeoutError) transport.reset(instanceId)
      }
    }
  }

  return {
    consumer: flow.id,
    upToOffset: computeUpToOffset(batch),
    outputs,
    feedSnapshots: [...snapshots.values()],
    discards,
    nodeRuns: [...nodeRuns.entries()].map(([nodeId, a]) => ({
      flowId: flow.id,
      nodeId,
      ok: a.ok,
      inCount: a.inCount,
      outCount: a.outCount,
      dropCount: a.dropCount,
      err: a.err,
      durMs: Math.round(a.durMs),
    })),
  }
}

// A github-source entry node only ingests its own flow-qualified topic; any
// other entry node (a test's bare processor with no upstream source) accepts
// the whole batch.
function acceptsEntry(flowId: string, node: FlowNode, msg: Msg): boolean {
  if (node.type !== githubSourceNode.type) return true
  return msg.Topic === `source:${flowId}/${node.id}`
}

/**
 * NodeResult's Msg[] and port-indexed-array shapes overlap syntactically
 * (both are plain arrays) — disambiguated by the node's declared output
 * count (from its own config.ts), not by inspecting the array's contents:
 * a 1-output node's array return is always "multiple messages, port 0"; an
 * N>1-output node's array return is always port-indexed (array[i] is
 * Msg | Msg[] | null for port i).
 */
function normalizeResult(result: NodeResult, outputs: number): Array<{ port: number; msg: Msg }> {
  if (result === null || result === undefined) return []
  if (!Array.isArray(result)) return [{ port: 0, msg: result }]
  if (outputs <= 1) {
    return (result as Array<Msg | null>).filter((m): m is Msg => m != null).map((msg) => ({ port: 0, msg }))
  }
  const out: Array<{ port: number; msg: Msg }> = []
  result.forEach((entry, port) => {
    if (entry == null) return
    if (Array.isArray(entry)) {
      for (const msg of entry) if (msg != null) out.push({ port, msg })
    } else {
      out.push({ port, msg: entry })
    }
  })
  return out
}

// Known processor types' output arity lives in their own config.ts
// (single source of truth); everything else defaults to 1 output. This is
// a small closed-form map rather than a Vite-glob registry because Phase 6
// (the registry-driven palette) is out of scope here — see processors.ts
// for the worker runtime registry, which only needs runtime.ts, not arity.
function outputCount(node: FlowNode): number {
  switch (node.type) {
    case functionNode.type:
      return functionNode.outputs(node.config as functionNode.Config)
    case githubFilterNode.type:
      return githubFilterNode.outputs
    default:
      return 1
  }
}

function forward(
  pending: Map<string, RuntimeMsg[]>,
  outWires: Map<string, Map<number, Wire[]>>,
  nodeId: string,
  port: number,
  msg: RuntimeMsg,
  run: NodeRunAcc,
  discards: Discard[],
): void {
  const wires = outWires.get(nodeId)?.get(port) ?? []
  if (wires.length === 0) {
    run.dropCount++
    discards.push({ msgId: msg.ID, nodeId })
    return
  }
  run.outCount++
  // Fan-out: every wire gets its own independent structuredClone so one
  // downstream branch mutating msg.payload can never affect a sibling
  // branch (or this node's own now-stale reference). A single wire needs
  // no clone.
  wires.forEach((wire) => {
    pushPending(pending, wire.to, wires.length > 1 ? structuredClone(msg) : msg)
  })
}

function pushPending(pending: Map<string, RuntimeMsg[]>, nodeId: string, msg: RuntimeMsg): void {
  const list = pending.get(nodeId)
  if (list) list.push(msg)
  else pending.set(nodeId, [msg])
}

function seedState(states: Map<string, Record<string, any>>, instanceId: string): Record<string, any> {
  const fresh: Record<string, any> = {}
  states.set(instanceId, fresh)
  return fresh
}

function acc(map: Map<string, NodeRunAcc>, nodeId: string): NodeRunAcc {
  let a = map.get(nodeId)
  if (!a) {
    a = { inCount: 0, outCount: 0, dropCount: 0, ok: true, err: '', durMs: 0 }
    map.set(nodeId, a)
  }
  return a
}

function nowMs(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

function computeUpToOffset(batch: Msg[]): string {
  if (batch.length === 0) return '0'
  let max = BigInt(batch[0].ID)
  for (const msg of batch) {
    const offset = BigInt(msg.ID)
    if (offset > max) max = offset
  }
  return max.toString()
}
