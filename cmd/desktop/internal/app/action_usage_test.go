package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
)

func TestActionUsageCheckerBlocksLoadedFlowsAndNonterminalQueueOnly(t *testing.T) {
	actionStore := actions.NewActionStore(filepath.Join(t.TempDir(), "actions.yml"))
	_, err := actionStore.Create(serviceAction("used"))
	require.NoError(t, err)
	flows := flow.NewFlowStore(t.TempDir(), actions.NewRefs(actionStore))
	f := flow.Flow{ID: "flow-a", Name: "Flow A", Enabled: true, Nodes: []flow.Node{
		{ID: "source", Type: ghsource.Descriptor.Type, Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: "github/octocat", Kind: "search", Query: "is:open"})},
		{ID: "action", Type: "action", Config: &flow.ActionConfig{Action: "used"}},
	}, Wires: []flow.Wire{{From: "source", To: "action"}}}
	require.NoError(t, flows.Save(f))

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, status := range []string{"pending", "running", "done", "failed"} {
		_, err := db.Conn().ExecContext(t.Context(), `INSERT INTO output_command (action_id, key, payload, status, created_at) VALUES (?, ?, ?, ?, 1)`, "used", status, []byte("{}"), status)
		require.NoError(t, err)
	}

	checker := newActionUsage(flows, stores.New(db, stores.Options{}).OutputCommands)
	usage, err := checker.Usage(t.Context(), "used")
	require.NoError(t, err)
	assert.Equal(t, []string{"flow-a"}, usage.FlowIDs)
	assert.EqualValues(t, 2, usage.ActiveCommands)

	actionStore.SetUsageChecker(checker)
	err = actionStore.Delete(t.Context(), "used")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow-a")
	assert.Contains(t, err.Error(), "2 nonterminal output command")

	// Terminal history alone is explicitly allowed once the deployed flow is
	// removed and all queue work has completed.
	require.NoError(t, flows.Delete("flow-a"))
	_, err = db.Conn().ExecContext(t.Context(), `UPDATE output_command SET status = 'done' WHERE status IN ('pending', 'running')`)
	require.NoError(t, err)
	require.NoError(t, actionStore.Delete(t.Context(), "used"))
}
