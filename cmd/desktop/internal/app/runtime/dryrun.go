package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// maxConsoleLines caps how many console lines one dry run collects, across
// every node. A script that logs per message would otherwise let a response
// grow with the input; the cap is reported rather than applied silently.
const maxConsoleLines = 1000

// DryRunResult is not a models.CommitBatch, so callers cannot accidentally
// commit a preview.
type DryRunResult struct {
	// Nodes is one entry per node a message reached, in execution order.
	Nodes []NodeTrace `json:"nodes"`
	// Outputs is what the terminals would have committed — feed memberships,
	// notifications and enqueued actions — had this been a live run.
	Outputs []models.Output `json:"outputs"`
	// FeedSnapshots is the reconciliation scope each snapshot input declared.
	// An empty snapshot still produces one: it is how a clean poll clears a
	// feed, so a dry run has to show it.
	FeedSnapshots []models.FeedSnapshot `json:"feedSnapshots"`
	// KVMutations is what the run would have written to durable node KV. The
	// values are the sandbox's, so this reads as "given the kv you seeded, here
	// is what the flow would store".
	KVMutations []models.KVMutation `json:"kvMutations"`
	// ConsoleTruncated reports that the run produced more console output than
	// it kept.
	ConsoleTruncated bool `json:"consoleTruncated,omitempty"`
}

// NodeTrace is what one node did during a dry run.
type NodeTrace struct {
	NodeID string `json:"nodeId"`
	Type   string `json:"type"`
	// Disabled marks a node that dropped everything without running, which is
	// otherwise indistinguishable from a node that chose to drop everything.
	Disabled bool `json:"disabled,omitempty"`
	// In, Out and Dropped are the same counters a live run records: messages
	// received, messages emitted or committed, and messages this node ended.
	In      int `json:"in"`
	Out     int `json:"out"`
	Dropped int `json:"dropped"`
	// OK is false when at least one message failed here.
	OK bool `json:"ok"`
	// Error is the last failure, structured when the node is a script.
	Error *NodeError `json:"error,omitempty"`
	// DurationMs is the total time spent in this node's processor.
	DurationMs int64 `json:"durationMs"`
	// Received is every message that arrived at this node's input, snapshots
	// already expanded into their items.
	Received []models.Msg `json:"received"`
	// Emitted is what the node put on each of its output ports, in port order.
	// A port with no wire behind it is still reported — what the node produced
	// does not depend on what is listening. Terminals emit nothing; read
	// Received and the run's Outputs for those.
	Emitted []PortEmission `json:"emitted"`
	// Console is the node's console output, oldest first.
	Console []ConsoleLine `json:"console,omitempty"`
}

// PortEmission is one output port's messages.
type PortEmission struct {
	Port     int          `json:"port"`
	Messages []models.Msg `json:"messages"`
}

// NodeError is a node failure as a dry run reports it. Kind, Line and Column
// are set only for a script failure — everything else carries Message alone.
type NodeError struct {
	Message string          `json:"message"`
	Kind    ScriptErrorKind `json:"kind,omitempty"`
	Line    int             `json:"line,omitempty"`
	Column  int             `json:"column,omitempty"`
	Stack   string          `json:"stack,omitempty"`
}

// ConsoleLine is one console.* call from a function node.
type ConsoleLine struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// DryRun executes the flow over messages injected at entryNodeID and reports
// what every node downstream of it did, without producing anything the caller
// could commit.
//
// Injection is by node id rather than by topic on purpose: it is what lets one
// function node be exercised against a captured payload without standing up
// the source in front of it. A source node is a relay at run time — the core
// ingests for it — so injecting at one is the same as that source having
// polled and returned exactly these messages, and nothing fetches.
//
// The run is as read-only as the ports it is given: durable KV is whatever
// Options.KV reads (seed a MemoryKV to sandbox it), and the mutations and
// outputs it would have produced come back in the result instead of being
// applied. Call it on a Runner built for this run alone — a Runner carries each
// function node's `state` object between messages, so dry-running through the
// engine's installed one would mutate live node state.
func (r *Runner) DryRun(ctx context.Context, entryNodeID string, batch []models.Msg) (DryRunResult, error) {
	if r.graph.Node(entryNodeID) == nil {
		return DryRunResult{}, fmt.Errorf("flow %q has no node %q to inject at", r.flow.ID, entryNodeID)
	}

	state := newRunState(r, newKVBuffer(r.opts.KV, r.flow.ID, time.Now().UnixMilli()))
	state.trace = &runTrace{}

	for _, msg := range batch {
		state.deliver([]string{entryNodeID}, msg)
	}
	if err := state.execute(ctx); err != nil {
		return DryRunResult{}, err
	}

	// Every collection is non-nil even when empty. A dry run is read by tooling
	// that filters over these — jq, a test, an agent — and `null` is the one
	// value that makes `| length` an error rather than zero.
	return DryRunResult{
		Nodes:            state.nodeTraces(),
		Outputs:          orEmpty(state.outputs),
		FeedSnapshots:    orEmpty(state.snapshots),
		KVMutations:      orEmpty(state.kv.mutations()),
		ConsoleTruncated: state.trace.truncated(),
	}, nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// nodeTraces assembles one trace per node that received a message, in the order
// they were first touched — the topological order restricted to the nodes that
// actually ran.
func (s *runState) nodeTraces() []NodeTrace {
	traces := make([]NodeTrace, 0, len(s.runOrder))
	for _, nodeID := range s.runOrder {
		acc := s.runs[nodeID]
		node := s.runner.graph.Node(nodeID)
		trace := NodeTrace{
			NodeID:     nodeID,
			In:         acc.inCount,
			Out:        acc.outCount,
			Dropped:    acc.dropCount,
			OK:         acc.ok,
			DurationMs: acc.dur.Milliseconds(),
			Received:   orEmpty(acc.received),
			Emitted:    portEmissions(acc.emitted),
			Console:    s.trace.linesFor(nodeID),
		}
		if node != nil {
			trace.Type, trace.Disabled = node.Type, node.Disabled
		}
		if acc.lastErr != nil {
			trace.Error = nodeErrorOf(acc.lastErr)
		}
		traces = append(traces, trace)
	}
	return traces
}

func portEmissions(byPort map[int][]models.Msg) []PortEmission {
	ports := make([]int, 0, len(byPort))
	for port := range byPort {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	out := make([]PortEmission, 0, len(ports))
	for _, port := range ports {
		out = append(out, PortEmission{Port: port, Messages: byPort[port]})
	}
	return out
}

func nodeErrorOf(err error) *NodeError {
	if scriptErr, ok := errors.AsType[*ScriptError](err); ok {
		return &NodeError{
			Message: scriptErr.Message,
			Kind:    scriptErr.Kind,
			Line:    scriptErr.Line,
			Column:  scriptErr.Column,
			Stack:   scriptErr.Stack,
		}
	}
	return &NodeError{Message: err.Error()}
}

// runTrace collects console output for a dry run.
//
// It takes a lock because a script that outlived its interrupt keeps running on
// a goroutine the engine has already abandoned, and that goroutine can still
// reach console.log while the run loop moves on to the next message. Everything
// else a trace records is written by the run loop alone and lives on the
// per-node accumulator.
type runTrace struct {
	mu      sync.Mutex
	lines   map[string][]ConsoleLine
	dropped bool
}

// consoleFor returns the sink for one node, or nil when there is no trace —
// which is every live run, and is why this is a method on a possibly-nil
// receiver rather than a branch at the call site.
func (t *runTrace) consoleFor(nodeID string) ConsoleSink {
	if t == nil {
		return nil
	}
	return func(level, text string) { t.add(nodeID, ConsoleLine{Level: level, Text: text}) }
}

func (t *runTrace) add(nodeID string, line ConsoleLine) {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := 0
	for _, lines := range t.lines {
		total += len(lines)
	}
	if total >= maxConsoleLines {
		t.dropped = true
		return
	}
	if t.lines == nil {
		t.lines = map[string][]ConsoleLine{}
	}
	t.lines[nodeID] = append(t.lines[nodeID], line)
}

func (t *runTrace) linesFor(nodeID string) []ConsoleLine {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lines[nodeID]
}

func (t *runTrace) truncated() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dropped
}

// MemoryKV is a KVReader over an in-memory seed: node id -> key -> the JSON
// text NodeKV stores. It is what makes a dry run's `kv` a sandbox — reads see
// only what the caller seeded, and the writes a run produces come back as
// mutations instead of reaching the database.
//
// Expiry is not modelled: a seed is a starting state a caller wrote by hand, so
// there is nothing older than the run itself to expire.
type MemoryKV map[string]map[string]string

func (m MemoryKV) Get(_ context.Context, _, nodeID, key string, _ int64) (string, bool, error) {
	value, ok := m[nodeID][key]
	return value, ok, nil
}

func (m MemoryKV) Keys(_ context.Context, _, nodeID, prefix string, _ int64) ([]string, error) {
	var keys []string
	for key := range m[nodeID] {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// SyntheticMsgID is the id given to an injected message the caller left
// unidentified, suffixed with its position in the batch. A message needs some
// id for the discards a drop records to name it; a dry run commits nothing, so
// unlike a live offset it only has to be distinct within the run.
func SyntheticMsgID(index int) string { return "dryrun-" + strconv.Itoa(index) }
