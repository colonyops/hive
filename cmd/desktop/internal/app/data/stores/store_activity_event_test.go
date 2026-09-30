package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func TestActivityEventStore_AppendAndList(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	first, err := st.ActivityEvents.Append(ctx, ActivityEventCreate{
		Category: "action", Severity: "info", Title: "first", Source: "test",
	})
	require.NoError(t, err)
	assert.NotZero(t, first.ID)
	assert.NotZero(t, first.CreatedAt)

	second, err := st.ActivityEvents.Append(ctx, ActivityEventCreate{
		Category: "action", Severity: "warn", Title: "second", Source: "test", Metadata: map[string]string{"k": "v"},
	})
	require.NoError(t, err)

	events, err := st.ActivityEvents.List(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, second.ID, events[0].ID, "newest first")
	assert.Equal(t, first.ID, events[1].ID)
	assert.Equal(t, map[string]string{"k": "v"}, events[0].Metadata)

	older, err := st.ActivityEvents.List(ctx, second.ID, 10)
	require.NoError(t, err)
	require.Len(t, older, 1)
	assert.Equal(t, first.ID, older[0].ID)
}

// A row with metadata no JSON parser accepts is a recoverable anomaly, not a
// failed page: List skips it and logs, rather than failing every other event
// on the page because of one bad row.
func TestActivityEventStore_ListSkipsUndecodableMetadata(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	good, err := st.ActivityEvents.Append(ctx, ActivityEventCreate{Category: "action", Severity: "info", Title: "good"})
	require.NoError(t, err)
	_, err = db.AppendActivityEvent(ctx, queries.AppendActivityEventParams{
		CreatedAt: 999, Category: "action", Severity: "info", Title: "corrupt", Metadata: []byte("not json"),
	})
	require.NoError(t, err)

	events, err := st.ActivityEvents.List(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1, "the undecodable row is skipped, not returned or failed")
	assert.Equal(t, good.ID, events[0].ID)
}
