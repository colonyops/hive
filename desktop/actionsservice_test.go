package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

func serviceAction(id string) actions.EditableAction {
	return actions.EditableAction{ID: id, Label: id, Type: "shell", ShowInDetail: true, Shell: &actions.EditableShellConfig{CommandTemplate: "true"}}
}

func newServiceStore(t *testing.T) (*actions.ActionStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions: []\n"), 0o600))
	return actions.NewActionStore(path), path
}

func TestActionsServiceSharedStoreCRUDGetAndSuccessfulWakeOnly(t *testing.T) {
	store, _ := newServiceStore(t)
	wakes := 0
	service := NewActionsService(store, func() { wakes++ })

	created, err := service.CreateAction(serviceAction("run"))
	require.NoError(t, err)
	assert.Equal(t, "run", created.ID)
	assert.Equal(t, 1, wakes)
	got, err := service.GetAction("run")
	require.NoError(t, err)
	assert.Equal(t, created, got)
	_, err = service.GetAction("missing")
	require.ErrorContains(t, err, "not found")

	updated := serviceAction("run")
	updated.Label = "Run now"
	_, err = service.UpdateAction("run", updated)
	require.NoError(t, err)
	assert.Equal(t, 2, wakes)
	assert.Equal(t, "Run now", store.ListEditable().Actions[0].Label, "service and runtime share one store")

	_, err = service.CreateAction(serviceAction("run"))
	require.Error(t, err)
	_, err = service.UpdateAction("other", updated)
	require.ErrorContains(t, err, "immutable")
	assert.Equal(t, 2, wakes)

	require.NoError(t, service.DeleteAction("run"))
	assert.Equal(t, 3, wakes)
	require.Error(t, service.DeleteAction("run"))
	assert.Equal(t, 3, wakes)
}

func TestActionsServiceListReturnsLastGoodActionsAndMalformedLatestError(t *testing.T) {
	store, path := newServiceStore(t)
	service := NewActionsService(store, nil)
	_, err := service.CreateAction(serviceAction("good"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions: ["), 0o600))
	require.Error(t, store.Reload())

	catalog := service.ListActions()
	require.Len(t, catalog.Actions, 1)
	assert.Equal(t, "good", catalog.Actions[0].ID)
	assert.Contains(t, catalog.Error, "actions")
}

func TestActionUsageCheckerBlocksLoadedFlowsAndNonterminalQueueOnly(t *testing.T) {
	store, _ := newServiceStore(t)
	_, err := store.Create(serviceAction("used"))
	require.NoError(t, err)
	flows := flow.NewFlowStore(t.TempDir(), newActionsRefs(store))
	f := flow.Flow{ID: "flow-a", Name: "Flow A", Enabled: true, Nodes: []flow.Node{
		{ID: "source", Type: "github-source", Config: &flow.GithubSourceConfig{Kind: "search", Query: "is:open"}},
		{ID: "action", Type: "action", Config: &flow.ActionConfig{Action: "used"}},
	}, Wires: []flow.Wire{{From: "source", To: "action"}}}
	require.NoError(t, flows.Save(f))

	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, status := range []string{"pending", "running", "awaiting_confirmation", "done", "failed"} {
		_, err := db.Conn().ExecContext(context.Background(), `INSERT INTO output_command (action_id, key, payload, status, created_at) VALUES (?, ?, ?, ?, 1)`, "used", status, []byte("{}"), status)
		require.NoError(t, err)
	}

	checker := actionUsageChecker{flows: flows, db: db}
	usage, err := checker.Usage("used")
	require.NoError(t, err)
	assert.Equal(t, []string{"flow-a"}, usage.FlowIDs)
	assert.EqualValues(t, 3, usage.ActiveCommands)

	store.SetUsageChecker(checker)
	err = store.Delete("used")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow-a")
	assert.Contains(t, err.Error(), "3 nonterminal output command")

	// Terminal history alone is explicitly allowed once the deployed flow is
	// removed and all queue work has completed.
	require.NoError(t, flows.Delete("flow-a"))
	_, err = db.Conn().ExecContext(context.Background(), `UPDATE output_command SET status = 'done' WHERE status IN ('pending', 'running', 'awaiting_confirmation')`)
	require.NoError(t, err)
	require.NoError(t, store.Delete("used"))
}
