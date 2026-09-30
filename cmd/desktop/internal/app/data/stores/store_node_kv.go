package stores

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

const KVScopeNode = "node"

type NodeKVStore struct {
	q   *queries.DB
	now func() time.Time
}

func NewNodeKVStore(q *queries.DB, opts Options) *NodeKVStore {
	return &NodeKVStore{q: q, now: opts.Now}
}

// now is a Unix-millisecond expiry cutoff shared by one tick; expires_at <=
// now is absent.
func (s *NodeKVStore) Get(ctx context.Context, flowID, nodeID, key string, now int64) (string, bool, error) {
	value, err := s.q.Ctx(ctx).GetNodeKV(ctx, queries.GetNodeKVParams{
		FlowID: flowID, NodeID: nodeID, Scope: KVScopeNode, Key: key, ExpiresAt: sql.NullInt64{Int64: now, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, wrap("reading node kv", err)
	}
	return value, true, nil
}

// Prefix matching must stay binary and case-sensitive to agree with the
// runtime's strings.HasPrefix overlay.
func (s *NodeKVStore) Keys(ctx context.Context, flowID, nodeID, prefix string, now int64) ([]string, error) {
	keys, err := s.q.Ctx(ctx).ListNodeKVKeysByPrefix(ctx, queries.ListNodeKVKeysByPrefixParams{
		FlowID: flowID, NodeID: nodeID, Scope: KVScopeNode,
		Pattern: escapeGlob(prefix) + "*",
		Now:     sql.NullInt64{Int64: now, Valid: true},
	})
	return keys, wrap("listing node kv keys", err)
}

// value is opaque JSON text; expiresAt is Unix milliseconds, with 0 meaning
// no expiry.
func (s *NodeKVStore) Set(ctx context.Context, flowID, nodeID, key, value string, expiresAt int64) error {
	var exp sql.NullInt64
	if expiresAt > 0 {
		exp = sql.NullInt64{Int64: expiresAt, Valid: true}
	}
	return wrap("writing node kv", s.q.Ctx(ctx).UpsertNodeKV(ctx, queries.UpsertNodeKVParams{
		FlowID: flowID, NodeID: nodeID, Scope: KVScopeNode, Key: key,
		Value: value, ExpiresAt: exp, UpdatedAt: s.now().UnixMilli(),
	}))
}

func (s *NodeKVStore) Delete(ctx context.Context, flowID, nodeID, key string) error {
	return wrap("deleting node kv", s.q.Ctx(ctx).DeleteNodeKV(ctx, queries.DeleteNodeKVParams{
		FlowID: flowID, NodeID: nodeID, Scope: KVScopeNode, Key: key,
	}))
}

func (s *NodeKVStore) DeleteByFlow(ctx context.Context, flowID string) error {
	return wrap("deleting node kv by flow", s.q.Ctx(ctx).DeleteNodeKVByFlow(ctx, flowID))
}

func (s *NodeKVStore) DeleteForFlowExceptNodes(ctx context.Context, flowID string, keepNodeIDs []string) error {
	return wrap("removing obsolete node kv", s.q.Ctx(ctx).DeleteNodeKVForFlowExceptNodes(ctx, queries.DeleteNodeKVForFlowExceptNodesParams{
		FlowID: flowID, NodeIds: keepNodeIDs,
	}))
}

// escapeGlob neutralizes GLOB metacharacters so a stored key's literal
// prefix always matches itself. `*` and `?` wrap in a character class; `[`
// is escaped the only way GLOB allows -- opening a class that contains it.
func escapeGlob(s string) string {
	replacer := strings.NewReplacer("*", "[*]", "?", "[?]", "[", "[[]")
	return replacer.Replace(s)
}
