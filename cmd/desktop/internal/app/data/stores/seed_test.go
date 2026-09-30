package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeed_WritesExactFieldsNoStoreMethodAllows(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	seed := NewSeed(db)

	item, err := seed.InboxItem(ctx, InboxItem{
		ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "seeded-1", URL: "https://example.test/1",
		Payload: []byte(`{}`), Unread: true, Lifecycle: "active", FirstSeenAt: 1_700_000_000_000, LastEventAt: 1_700_000_001_000,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1_700_000_000_000), item.FirstSeenAt, "the fixture's exact timestamp survives, not the store's own clock")
	assert.Equal(t, "https://example.test/1", item.URL)
	assert.True(t, item.Unread)

	event, err := seed.InboxEvent(ctx, InboxEvent{
		ItemID: item.ID, Kind: "observed", Transition: "none", Attention: "trivial", Summary: "seen", Detail: []byte(`{}`), CreatedAt: 1_700_000_002_000,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1_700_000_002_000), event.CreatedAt)
	assert.Equal(t, "seen", event.Summary)

	require.NoError(t, seed.ConsumerOffset(ctx, "consumer-a", 5))
	offset, err := st.EventLog.ConsumerOffset(ctx, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset)
}
