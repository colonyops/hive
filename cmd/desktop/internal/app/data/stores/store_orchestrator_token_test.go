package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestratorTokenStore(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	created, err := st.OrchestratorTokens.Create(ctx, "worker", "hash-1", "ab12")
	require.NoError(t, err)
	assert.Zero(t, created.LastUsedAt)

	got, err := st.OrchestratorTokens.GetByHash(ctx, "hash-1")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	require.NoError(t, st.OrchestratorTokens.Touch(ctx, created.ID))
	list, err := st.OrchestratorTokens.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.NotZero(t, list[0].LastUsedAt)

	removed, err := st.OrchestratorTokens.Delete(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, removed)
	removed, err = st.OrchestratorTokens.Delete(ctx, created.ID)
	require.NoError(t, err)
	assert.False(t, removed)

	_, err = st.OrchestratorTokens.GetByHash(ctx, "hash-1")
	assert.True(t, IsNotFound(err))
}
