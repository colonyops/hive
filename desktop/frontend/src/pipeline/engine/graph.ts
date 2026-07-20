// Pure graph helpers shared by runGraph: in-degree/entry-node discovery,
// per-port outbound wire indexing, and a topological sort used both to
// drive execution order and as a cycle guard.

import type { Flow, Wire } from '../types'

/** Number of inbound wires per node id (0 = entry node candidate). Wires with either endpoint unknown are ignored here; Go SaveFlow validation owns ref errors. */
export function inDegrees(flow: Flow): Map<string, number> {
  const known = new Set(flow.nodes.map((n) => n.id))
  const deg = new Map<string, number>()
  for (const node of flow.nodes) deg.set(node.id, 0)
  for (const wire of flow.wires) {
    if (known.has(wire.from) && known.has(wire.to)) deg.set(wire.to, (deg.get(wire.to) ?? 0) + 1)
  }
  return deg
}

/** nodeId -> port -> wires leaving that node on that port (out defaults to 0). Wires to/from unknown node ids are dropped. */
export function outWiresByPort(flow: Flow): Map<string, Map<number, Wire[]>> {
  const known = new Set(flow.nodes.map((n) => n.id))
  const index = new Map<string, Map<number, Wire[]>>()
  for (const wire of flow.wires) {
    if (!known.has(wire.from) || !known.has(wire.to)) continue
    const port = wire.out ?? 0
    let byPort = index.get(wire.from)
    if (!byPort) {
      byPort = new Map()
      index.set(wire.from, byPort)
    }
    const list = byPort.get(port)
    if (list) list.push(wire)
    else byPort.set(port, [wire])
  }
  return index
}

/**
 * Kahn's algorithm. Flow validation (cycle diagnostics, dangling refs,
 * port-bounds) is Go SaveFlow validation's job — runGraph assumes an acyclic flow, but
 * still guards against hanging forever on a bad one: if a cycle sneaks in,
 * some nodes never reach in-degree 0 and get left out of `order`, so we
 * throw instead of looping.
 */
export function topoSort(flow: Flow): string[] {
  const remaining = inDegrees(flow)
  const outWires = outWiresByPort(flow)

  const queue: string[] = []
  for (const [id, deg] of remaining) if (deg === 0) queue.push(id)

  const order: string[] = []
  for (let i = 0; i < queue.length; i++) {
    const id = queue[i]
    order.push(id)
    const byPort = outWires.get(id)
    if (!byPort) continue
    for (const wires of byPort.values()) {
      for (const wire of wires) {
        const deg = (remaining.get(wire.to) ?? 0) - 1
        remaining.set(wire.to, deg)
        if (deg === 0) queue.push(wire.to)
      }
    }
  }

  if (order.length !== flow.nodes.length) {
    throw new Error(`pipeline flow "${flow.id}" is not a DAG (cycle detected) — cannot execute`)
  }
  return order
}
