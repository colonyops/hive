package runtime

import (
	"context"
	"maps"
	"sort"
	"strings"
	"sync"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// KVReader provides durable node-scoped reads. Callers pass one
// Unix-millisecond cutoff per Run so expiry is deterministic.
type KVReader interface {
	Get(ctx context.Context, flowID, nodeID, key string, now int64) (value string, found bool, err error)
	Keys(ctx context.Context, flowID, nodeID, prefix string, now int64) ([]string, error)
}

// NodeKV is the node-scoped surface a function node's `kv` object is bound
// to. Values are opaque JSON text; the js layer owns (de)serialization.
type NodeKV interface {
	Get(ctx context.Context, key string) (value string, found bool, err error)
	Set(ctx context.Context, key, value string, ttlSeconds int64) error
	Has(ctx context.Context, key string) (bool, error)
	Delete(ctx context.Context, key string) error
	Keys(ctx context.Context, prefix string) ([]string, error)
}

type kvOp struct {
	deleted   bool
	value     string
	expiresAt int64 // unix ms, 0 = no expiry
}

// kvBuffer is one Run's KV working set, owned by the run loop. When inert
// (replay), reads miss and writes are discarded in both the durable and
// overlay directions, so replay never consults or produces dedup history.
type kvBuffer struct {
	reader KVReader
	flowID string
	now    int64
	inert  bool

	// mu guards ops. The run loop is single-threaded, but a wedged script's
	// abandoned goroutine can still hold a staging handle whose reads reach
	// ops while the loop merges a later message's staging. The ctx guard on
	// every staging method makes that window unreachable in any realistic
	// schedule; the mutex removes the formal race outright. Each staging's
	// own staged map needs no lock — only its evaluating goroutine touches
	// it, and an errored (or wedged) message's staging is never merged.
	mu  sync.Mutex
	ops map[string]map[string]kvOp // nodeID -> key -> merged op
}

func newKVBuffer(r KVReader, flowID string, now int64) *kvBuffer {
	if r == nil {
		r = noopKVReader{}
	}
	return &kvBuffer{reader: r, flowID: flowID, now: now, ops: map[string]map[string]kvOp{}}
}

func newInertKVBuffer(flowID string, now int64) *kvBuffer {
	b := newKVBuffer(noopKVReader{}, flowID, now)
	b.inert = true
	return b
}

func (b *kvBuffer) node(nodeID string) *nodeStaging {
	return &nodeStaging{buf: b, nodeID: nodeID, staged: map[string]kvOp{}}
}

// mutations drains the merged writes, sorted by (nodeID, key) for
// deterministic batches.
func (b *kvBuffer) mutations() []models.KVMutation {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inert || len(b.ops) == 0 {
		return nil
	}
	nodeIDs := make([]string, 0, len(b.ops))
	for nodeID := range b.ops {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)

	var out []models.KVMutation
	for _, nodeID := range nodeIDs {
		ops := b.ops[nodeID]
		keys := make([]string, 0, len(ops))
		for key := range ops {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			op := ops[key]
			out = append(out, models.KVMutation{
				NodeID:    nodeID,
				Key:       key,
				Delete:    op.deleted,
				Value:     op.value,
				ExpiresAt: op.expiresAt,
			})
		}
	}
	return out
}

// nodeStaging layers this message's writes over prior writes and durable state.
// A message's writes merge only after successful execution.
type nodeStaging struct {
	buf    *kvBuffer
	nodeID string
	staged map[string]kvOp
}

func (s *nodeStaging) commit() {
	if s.buf.inert || len(s.staged) == 0 {
		return
	}
	s.buf.mu.Lock()
	defer s.buf.mu.Unlock()
	merged := s.buf.ops[s.nodeID]
	if merged == nil {
		merged = map[string]kvOp{}
		s.buf.ops[s.nodeID] = merged
	}
	maps.Copy(merged, s.staged)
}

func (s *nodeStaging) Get(ctx context.Context, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if s.buf.inert {
		return "", false, nil
	}
	if op, ok := s.staged[key]; ok {
		return s.resolve(op)
	}
	s.buf.mu.Lock()
	op, merged := s.buf.ops[s.nodeID][key]
	s.buf.mu.Unlock()
	if merged {
		return s.resolve(op)
	}
	return s.buf.reader.Get(ctx, s.buf.flowID, s.nodeID, key, s.buf.now)
}

func (s *nodeStaging) Has(ctx context.Context, key string) (bool, error) {
	_, found, err := s.Get(ctx, key)
	return found, err
}

func (s *nodeStaging) Set(ctx context.Context, key, value string, ttlSeconds int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.buf.inert {
		return nil
	}
	var expiresAt int64
	if ttlSeconds > 0 {
		expiresAt = s.buf.now + ttlSeconds*1000
	}
	s.staged[key] = kvOp{value: value, expiresAt: expiresAt}
	return nil
}

func (s *nodeStaging) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.buf.inert {
		return nil
	}
	s.staged[key] = kvOp{deleted: true}
	return nil
}

// Keys matches with strings.HasPrefix — binary and case-sensitive, the same
// semantics as the durable GLOB query.
func (s *nodeStaging) Keys(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.buf.inert {
		return nil, nil
	}
	durable, err := s.buf.reader.Keys(ctx, s.buf.flowID, s.nodeID, prefix, s.buf.now)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(durable))
	for _, key := range durable {
		seen[key] = true
	}
	overlay := func(ops map[string]kvOp) {
		for key, op := range ops {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			if op.deleted || s.expired(op) {
				delete(seen, key)
			} else {
				seen[key] = true
			}
		}
	}
	s.buf.mu.Lock()
	overlay(s.buf.ops[s.nodeID])
	s.buf.mu.Unlock()
	overlay(s.staged)

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *nodeStaging) resolve(op kvOp) (string, bool, error) {
	if op.deleted || s.expired(op) {
		return "", false, nil
	}
	return op.value, true, nil
}

func (s *nodeStaging) expired(op kvOp) bool {
	return op.expiresAt > 0 && op.expiresAt <= s.buf.now
}

type noopKVReader struct{}

func (noopKVReader) Get(context.Context, string, string, string, int64) (string, bool, error) {
	return "", false, nil
}

func (noopKVReader) Keys(context.Context, string, string, string, int64) ([]string, error) {
	return nil, nil
}
