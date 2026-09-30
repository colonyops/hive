package stores

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func TestNodeKV_SetGetRoundTrip(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "seen", `{"state":"open"}`, 0))

	value, found, err := st.NodeKV.Get(ctx, "flow-1", "dedup", "seen", 1000)
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, `{"state":"open"}`, value)

	// An upsert replaces the stored value in place.
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "seen", `{"state":"merged"}`, 0))
	value, found, err = st.NodeKV.Get(ctx, "flow-1", "dedup", "seen", 1000)
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, `{"state":"merged"}`, value)
}

func TestNodeKV_MissingKeyReadsAbsent(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	value, found, err := st.NodeKV.Get(ctx, "flow-1", "dedup", "missing", 1000)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestNodeKV_Delete(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "seen", `true`, 0))
	require.NoError(t, st.NodeKV.Delete(ctx, "flow-1", "dedup", "seen"))

	_, found, err := st.NodeKV.Get(ctx, "flow-1", "dedup", "seen", 1000)
	require.NoError(t, err)
	assert.False(t, found)

	// Deleting a key that never existed is a no-op, not an error.
	require.NoError(t, st.NodeKV.Delete(ctx, "flow-1", "dedup", "never"))
}

// Two nodes in one flow never see each other's keys: node_id is part of the
// composite primary key, so the same key name under different nodes holds
// independent rows.
func TestNodeKV_CrossNodeKeyIsolation(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "node-a", "x", `1`, 0))

	_, found, err := st.NodeKV.Get(ctx, "flow-1", "node-b", "x", 1000)
	require.NoError(t, err)
	assert.False(t, found)

	// And the same node id under a different flow is just as invisible.
	_, found, err = st.NodeKV.Get(ctx, "flow-2", "node-a", "x", 1000)
	require.NoError(t, err)
	assert.False(t, found)
}

// The expiry boundary is pinned deterministically with a fixed cutoff: a row
// expiring exactly at `now` reads absent (the comparison is strictly >), one
// expiring a millisecond later reads present. No wall clock involved.
func TestNodeKV_ExpiryBoundary(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	const now = int64(5000)

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "at-now", `1`, now))
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "after-now", `1`, now+1))

	_, found, err := st.NodeKV.Get(ctx, "flow-1", "dedup", "at-now", now)
	require.NoError(t, err)
	assert.False(t, found, "expires_at == now must read absent")

	_, found, err = st.NodeKV.Get(ctx, "flow-1", "dedup", "after-now", now)
	require.NoError(t, err)
	assert.True(t, found, "expires_at == now+1 must read present")

	// The same row becomes readable again only by rewriting it; a later
	// cutoff also expires the second row.
	_, found, err = st.NodeKV.Get(ctx, "flow-1", "dedup", "after-now", now+1)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestNodeKV_KeysByPrefix(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	const now = int64(1000)

	for _, key := range []string{"pr:1", "pr:2", "issue:1"} {
		require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", key, `1`, 0))
	}
	// An expired key under the prefix is not listed.
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "pr:expired", `1`, now))
	// Another node's key under the same prefix is not listed.
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "other", "pr:9", `1`, 0))

	keys, err := st.NodeKV.Keys(ctx, "flow-1", "dedup", "pr:", now)
	require.NoError(t, err)
	assert.Equal(t, []string{"pr:1", "pr:2"}, keys)

	all, err := st.NodeKV.Keys(ctx, "flow-1", "dedup", "", now)
	require.NoError(t, err)
	assert.Equal(t, []string{"issue:1", "pr:1", "pr:2"}, all)
}

// The prefix match is binary/case-sensitive, agreeing with Go's
// strings.HasPrefix — the default LIKE would be ASCII case-insensitive and
// silently disagree with the runtime overlay on a mixed-case prefix.
func TestNodeKV_KeysPrefixIsCaseSensitive(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "Foo", `1`, 0))
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "foo", `1`, 0))

	keys, err := st.NodeKV.Keys(ctx, "flow-1", "dedup", "foo", 1000)
	require.NoError(t, err)
	assert.Equal(t, []string{"foo"}, keys)
}

// GLOB metacharacters in a prefix are literals to the caller: a stored key
// containing *, ? or [ is matched by its own prefix and by nothing else.
func TestNodeKV_KeysEscapesGlobMetacharacters(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	for _, key := range []string{"a*b", "a?b", "a[b", "axb"} {
		require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", key, `1`, 0))
	}

	for prefix, want := range map[string][]string{
		"a*": {"a*b"},
		"a?": {"a?b"},
		"a[": {"a[b"},
	} {
		keys, err := st.NodeKV.Keys(ctx, "flow-1", "dedup", prefix, 1000)
		require.NoError(t, err)
		assert.Equal(t, want, keys, "prefix %q", prefix)
	}
}

func TestNodeKV_DeleteExpiredSweep(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	const now = int64(5000)

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "expired", `1`, now))
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "live", `1`, now+1))
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "forever", `1`, 0))

	require.NoError(t, db.Ctx(ctx).DeleteExpiredNodeKV(ctx, sql.NullInt64{Int64: now, Valid: true}))

	assert.Equal(t, 2, countNodeKVRows(t, db, "flow-1"))
	_, found, err := st.NodeKV.Get(ctx, "flow-1", "dedup", "forever", now)
	require.NoError(t, err)
	assert.True(t, found)
}

func TestNodeKV_DeleteByFlow(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "a", "k", `1`, 0))
	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "b", "k", `1`, 0))
	require.NoError(t, st.NodeKV.Set(ctx, "flow-2", "a", "k", `1`, 0))

	require.NoError(t, st.NodeKV.DeleteByFlow(ctx, "flow-1"))

	assert.Equal(t, 0, countNodeKVRows(t, db, "flow-1"))
	assert.Equal(t, 1, countNodeKVRows(t, db, "flow-2"), "another flow's rows are untouched")
}

func countNodeKVRows(t *testing.T, db *queries.DB, flowID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM node_kv WHERE flow_id = ?`, flowID).Scan(&n))
	return n
}
