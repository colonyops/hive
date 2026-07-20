// Wire types are re-exported from the generated Wails bindings so the wire
// contract has one source, mirroring src/types/feed.ts's convention. Engine
// and node code must import Msg/CommitBatch/Output/etc. from here, never
// from bindings/ directly.
//
// Sink/Output/Discard/NodeRun are only re-exported as top-level type aliases
// under the `pipeline` package's generated models.ts for the two names a
// bound service function signature actually references (CommitBatch,
// FeedItem) — the field types nested inside CommitBatch (Sink/Output/
// Discard/NodeRunView) are generated directly in pipelinedb's own
// models.ts, so that's where this pulls them from. NodeRun is exported
// there as NodeRunView (see pipeline/commit.go's NodeRun alias comment);
// re-exported here under the name callers actually use, same as the Go side.
export type { CommitBatch, FeedItem } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'
export type { Msg, Output, Sink, Discard, FeedSnapshot } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb/models'
export type { NodeRunView as NodeRun } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb/models'

import type { Discard, FeedSnapshot, NodeRunView as NodeRun, Output } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb/models'

// Flow model (TS). The engine operates on in-memory Flow objects supplied
// by the editor/session layer, which adapts the generated Wails flow model
// from Go's flows/*.yaml loader.
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

/**
 * CommitResult is runGraph's return shape. It mirrors CommitBatch field for
 * field (consumer/upToOffset/outputs/feedSnapshots/discards/nodeRuns) and is structurally
 * assignable to it — a caller can pass a CommitResult straight into the
 * generated Commit() binding — but is declared separately so the engine has
 * its own name to document rather than reusing the wire type as a return
 * type.
 */
export interface CommitResult {
  consumer: string
  /** Decimal event-log offset. Strings preserve SQLite int64 precision in JavaScript. */
  upToOffset: string
  outputs: Output[]
  feedSnapshots: FeedSnapshot[]
  discards: Discard[]
  nodeRuns: NodeRun[]
}
