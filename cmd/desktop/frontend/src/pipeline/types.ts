// The editor's flow model. It is the shape the canvas, the drawer and the
// palette work with — a node's per-type fields nested under `config` — as
// opposed to the flattened wire shape GetFlow/SaveFlow speak, which
// lib/wireFlow.ts converts to and from.
//
// The commit protocol's types (Msg, CommitBatch, Output, Sink, …) used to be
// re-exported here, because the graph ran in this process. It runs in Go now
// (internal/app/runtime, ADR flow-engine-in-go), so nothing on this side constructs a
// commit and there is no wire contract left to mirror.

export interface FlowNode {
  id: string
  type: string
  /** Author-facing display name edited in the drawer; nodes without one fall back to their type's label. */
  name?: string
  disabled?: boolean
  config: Record<string, any>
}

// out defaults to 0 (single-output nodes never need to set it).
export interface Wire {
  from: string
  out?: number
  to: string
}

export interface Flow {
  id: string
  nodes: FlowNode[]
  wires: Wire[]
}
