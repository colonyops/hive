// Package runtime is the flow engine: it executes one flow over one batch of
// event-log messages and produces the CommitBatch that batch is worth.
//
// It runs in Go, in-process, and it is the whole of graph execution —
// topological order, port routing, filter matching, script evaluation,
// terminal sink tagging and node-run accounting. Nothing about it is
// transport-specific, which is the point: the editor preview, a CLI dry-run
// and an MCP tool all reach the same execution rather than the desktop window
// owning one of them.
//
// The engine never writes the database and never opens its own connection.
// Run takes the messages it is given, reads durable node KV only through the
// KVReader port the caller supplies, and returns what should be committed —
// including buffered KV writes — for the caller to apply. That keeps a
// dry-run (no-op reader, discarded batch) and a live tick the same code
// path, differing only in what the ports are wired to.
package runtime

import (
	"fmt"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// UnroutedNodeID is the Discard.NodeID recorded for a message that matched no
// entry node — its topic belongs to another flow's source, say. It is still
// accounted for, because a message nobody claims must not stop the consumer
// offset advancing past it.
const UnroutedNodeID = "$unrouted"

// Options configure a Runner. The zero value is usable: it runs a flow with
// no script runtimes registered, which is correct for a flow with no function
// nodes and an explicit error for one that has them.
type Options struct {
	// Scripts is the registry function nodes resolve their language through.
	Scripts *ScriptRegistry
	// KV is the durable node-KV read port; nil means every read misses
	// (preview, dry-run, tests).
	KV KVReader
}

// Runner executes one flow, repeatedly. It owns the per-node-instance state
// that has to survive between batches — a function node's `state` object,
// most importantly — so a flow that is pumped page by page behaves as one
// continuous run rather than resetting on every page.
//
// A Runner is not safe for concurrent use. One flow is one consumer of one
// ordered log, so running two batches of the same flow at once would reorder
// it; the caller serialises.
type Runner struct {
	flow  flow.Flow
	graph *Graph
	opts  Options

	// procs holds one processor per processing node, built once here rather
	// than per batch: a filter's globs and a script's compiled program are
	// worth keeping.
	procs map[string]processor
	// timeouts is each processing node's evaluation budget, resolved from its
	// own config at build time.
	timeouts map[string]time.Duration
}

// NewRunner indexes f and builds its processors. It fails on a flow that is
// not a DAG, on a node type the engine cannot execute, and on a function node
// whose script does not compile — all three are conditions under which
// running the flow at all would be worse than reporting it.
func NewRunner(f flow.Flow, opts Options) (*Runner, error) {
	graph, err := NewGraph(f)
	if err != nil {
		return nil, err
	}

	r := &Runner{
		flow:     f,
		graph:    graph,
		opts:     opts,
		procs:    map[string]processor{},
		timeouts: map[string]time.Duration{},
	}

	for i := range f.Nodes {
		node := &f.Nodes[i]
		b, known := behaviors[node.Type]
		if !known {
			r.Close()
			return nil, fmt.Errorf("flow %q: node %q: the engine cannot execute type %q", f.ID, node.ID, node.Type)
		}
		if b.processor == nil {
			continue
		}
		proc, err := b.processor(r, node.ID, node.Config)
		if err != nil {
			r.Close()
			return nil, fmt.Errorf("flow %q: node %q: %w", f.ID, node.ID, err)
		}
		r.procs[node.ID] = proc
		r.timeouts[node.ID] = r.nodeTimeout(node.Config)
	}
	return r, nil
}

// Close releases every processor's resources.
func (r *Runner) Close() {
	for id, proc := range r.procs {
		proc.close()
		delete(r.procs, id)
	}
}

// resetProcessors drops each processor's accumulated in-memory state while
// keeping the runner usable. RunReplay calls it after the recompute so
// replay-mutated state never reaches the first live pump.
func (r *Runner) resetProcessors() {
	for _, proc := range r.procs {
		proc.reset()
	}
}

// nodeTimeout resolves a node's evaluation budget from its own config,
// falling back to flow.DefaultFunctionTimeout.
func (r *Runner) nodeTimeout(cfg flow.NodeConfig) time.Duration {
	if fn, ok := cfg.(*flow.FunctionConfig); ok {
		return fn.EffectiveTimeout()
	}
	return flow.DefaultFunctionTimeout
}
