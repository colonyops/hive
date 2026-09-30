package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func ingestActiveSourceHead(t *testing.T, st *Stores, topic, profileID, sourceScope, externalID string) {
	t.Helper()
	_, err := st.InboxItems.IngestObservation(t.Context(), activityClassifier(externalID), IngestObservationParams{
		ProfileID: profileID,
		Topic:     topic,
		Current: models.Observation{
			ExternalID: externalID, Title: externalID, SourceKind: "github", SourceScope: sourceScope,
			ObservedAt: 1, Payload: []byte(`{"v":1}`),
		},
	})
	require.NoError(t, err)
}

func TestSourceHeadStore_ListActiveKeys(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	const topic = "source:profile/source"

	ingestActiveSourceHead(t, st, topic, "profile", "main", "active-item")

	ingestActiveSourceHead(t, st, topic, "profile", "main", "archived-item")
	_, err := db.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = 1, archived_actor = 'manual' WHERE external_id = ?`, "archived-item")
	require.NoError(t, err)

	ingestActiveSourceHead(t, st, topic, "profile", "main", "orphaned-item")
	_, err = db.Conn().ExecContext(ctx, `DELETE FROM inbox_item WHERE external_id = ?`, "orphaned-item")
	require.NoError(t, err)

	ingestActiveSourceHead(t, st, topic, "profile", "other-scope", "wrong-scope-item")

	keys, err := st.SourceHeads.ListActiveKeys(ctx, SourceIdentity{
		Topic: topic, ProfileID: "profile", SourceKind: "github", SourceScope: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"active-item"}, keys)
}

func TestSourceHeadStore_UpsertPayloadDelete(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:test", "key-1", []byte(`{"v":1}`)))
	payload, err := st.SourceHeads.Payload(ctx, "source:test", "key-1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v":1}`, string(payload))

	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:test", "key-1", []byte(`{"v":2}`)))
	payload, err = st.SourceHeads.Payload(ctx, "source:test", "key-1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v":2}`, string(payload))

	require.NoError(t, st.SourceHeads.Delete(ctx, "source:test", "key-1"))
	_, err = st.SourceHeads.Payload(ctx, "source:test", "key-1")
	require.Error(t, err)
}

func TestSourceHeadStore_DeleteByTopicPrefix(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:flow-1/a", "k1", []byte(`{}`)))
	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:flow-1/b", "k2", []byte(`{}`)))
	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:flow-2/a", "k3", []byte(`{}`)))

	require.NoError(t, st.SourceHeads.DeleteByTopicPrefix(ctx, "source:flow-1/"))

	_, err := st.SourceHeads.Payload(ctx, "source:flow-1/a", "k1")
	require.Error(t, err)
	_, err = st.SourceHeads.Payload(ctx, "source:flow-1/b", "k2")
	require.Error(t, err)
	payload, err := st.SourceHeads.Payload(ctx, "source:flow-2/a", "k3")
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(payload))
}

// TestSourceHeadStore_DeleteByTopicPrefixTakesThePrefixLiterally guards the
// escaping: a profile id containing LIKE metacharacters must not widen the
// delete past that profile's own rows.
func TestSourceHeadStore_DeleteByTopicPrefixTakesThePrefixLiterally(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:flow_1/a", "k1", []byte(`{}`)))
	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:flowX1/a", "k2", []byte(`{}`)))
	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:flow%/a", "k3", []byte(`{}`)))

	require.NoError(t, st.SourceHeads.DeleteByTopicPrefix(ctx, "source:flow_1/"))

	_, err := st.SourceHeads.Payload(ctx, "source:flow_1/a", "k1")
	require.Error(t, err)
	_, err = st.SourceHeads.Payload(ctx, "source:flowX1/a", "k2")
	require.NoError(t, err, "an underscore in the prefix must not match any character")
	_, err = st.SourceHeads.Payload(ctx, "source:flow%/a", "k3")
	require.NoError(t, err, "a percent sign in a topic must not be treated as a wildcard")
}
