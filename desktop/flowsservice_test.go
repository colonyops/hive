package main

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowsServiceDeleteFlowPurgesPipelineStateAndRetriesMissingFiles(t *testing.T) {
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store := flow.NewFlowStore(t.TempDir(), nil)
	service := NewFlowsService(store, db)
	created, err := service.CreateFlow("Profile")
	require.NoError(t, err)
	_, err = db.Queries().InsertInboxItem(context.Background(), pipelinedb.InsertInboxItemParams{
		ProfileID: created.ID, SourceKind: "github", ExternalID: "item", Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)
	_, err = db.Append(context.Background(), "source:"+created.ID+"/source", "item", []byte(`{}`))
	require.NoError(t, err)

	require.NoError(t, service.DeleteFlow(created.ID))
	// The second call is the files-first retry path: the yaml file is already
	// gone, but PurgeProfile remains an idempotent no-op.
	require.NoError(t, service.DeleteFlow(created.ID))
	for _, table := range []string{"inbox_item", "event_log", "consumer_offset", "source_head"} {
		var count int
		require.NoError(t, db.Conn().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count))
		assert.Zero(t, count, table)
	}
}
