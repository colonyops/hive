package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
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

// newTestActionsService wires an ActionsService over a real bus, so a test
// can assert the events.ActionsUpdated payload rather than counting a bare
// callback.
func newTestActionsService(t *testing.T, store *actions.ActionStore) (*ActionsService, <-chan events.ActionsUpdated) {
	t.Helper()
	bus := newTestBus(t)
	ch := subscribeEvents[events.ActionsUpdated](t, bus)
	return newActionsService(store, bus), ch
}

func TestActionsServiceSharedStoreCRUDPublishesOnSuccessOnly(t *testing.T) {
	actionStore, _ := newServiceStore(t)
	service, ch := newTestActionsService(t, actionStore)

	created, err := service.Create(t.Context(), serviceAction("run"))
	require.NoError(t, err)
	assert.Equal(t, "run", created.ID)
	e := requireEvents(t, ch, 1)[0]
	assert.Equal(t, 1, e.Count)

	updated := serviceAction("run")
	updated.Label = "Run now"
	_, err = service.Update(t.Context(), "run", updated)
	require.NoError(t, err)
	e = requireEvents(t, ch, 1)[0]
	assert.Equal(t, 1, e.Count)
	assert.Equal(t, "Run now", actionStore.ListEditable().Actions[0].Label, "service and runtime share one actionStore")

	_, err = service.Create(t.Context(), serviceAction("run"))
	require.Error(t, err)
	_, err = service.Update(t.Context(), "other", updated)
	require.ErrorContains(t, err, "immutable")
	requireNoMoreEvents(t, ch)

	require.NoError(t, service.Delete(t.Context(), "run"))
	e = requireEvents(t, ch, 1)[0]
	assert.Equal(t, 0, e.Count)
	require.Error(t, service.Delete(t.Context(), "run"))
	requireNoMoreEvents(t, ch)
}

func TestActionsServiceReorderPublishesOnlyOnAcceptedOrders(t *testing.T) {
	actionStore, _ := newServiceStore(t)
	service, ch := newTestActionsService(t, actionStore)
	for _, id := range []string{"one", "two"} {
		_, err := service.Create(t.Context(), serviceAction(id))
		require.NoError(t, err)
	}
	requireEvents(t, ch, 2)

	require.NoError(t, service.Reorder(t.Context(), []string{"two", "one"}))
	e := requireEvents(t, ch, 1)[0]
	assert.Equal(t, 2, e.Count)
	catalog := service.List(t.Context())
	require.Len(t, catalog.Actions, 2)
	assert.Equal(t, "two", catalog.Actions[0].ID)

	reorderErr := service.Reorder(t.Context(), []string{"two"})
	require.ErrorContains(t, reorderErr, "the catalog changed")
	assert.Equal(t, KindConflict, KindOf(reorderErr), "a stale catalog is the caller's view having moved")
	requireNoMoreEvents(t, ch)
}

func TestActionsServiceListReturnsLastGoodActionsAndMalformedLatestError(t *testing.T) {
	actionStore, path := newServiceStore(t)
	service, ch := newTestActionsService(t, actionStore)
	_, err := service.Create(t.Context(), serviceAction("good"))
	require.NoError(t, err)
	requireEvents(t, ch, 1)
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions: ["), 0o600))
	require.Error(t, actionStore.Reload())

	catalog := service.List(t.Context())
	require.Len(t, catalog.Actions, 1)
	assert.Equal(t, "good", catalog.Actions[0].ID)
	assert.Contains(t, catalog.Error, "actions")
}

func TestActionsServiceUpdateKeepsFlowReferencedActionsHeadless(t *testing.T) {
	actionStore, _ := newServiceStore(t)
	headless := actions.EditableAction{ID: "used", Label: "Used", Type: "launch-session", Launch: &actions.EditableLaunchConfig{
		PromptTemplate: "Review", RepoTemplate: "https://github.com/owner/repo.git",
	}}
	_, err := actionStore.Create(headless)
	require.NoError(t, err)
	flows := flow.NewFlowStore(t.TempDir(), actions.NewRefs(actionStore))
	require.NoError(t, flows.Save(flow.Flow{ID: "flow-a", Name: "Flow A", Enabled: true, Nodes: []flow.Node{
		{ID: "source", Type: ghsource.Descriptor.Type, Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: "github/octocat", Kind: "search", Query: "is:open"})},
		{ID: "action", Type: "action", Config: &flow.ActionConfig{Action: "used"}},
	}, Wires: []flow.Wire{{From: "source", To: "action"}}}))
	actionStore.SetUsageChecker(flowOnlyUsage{flows: flows})

	service, ch := newTestActionsService(t, actionStore)
	interactive := headless
	interactive.Launch = &actions.EditableLaunchConfig{PromptTemplate: "Review"}
	_, err = service.Update(t.Context(), "used", interactive)
	require.ErrorContains(t, err, "flow-a")
	requireNoMoreEvents(t, ch)

	// Existing flows do not prevent an update that remains headless.
	headless.Label = "Updated"
	_, err = service.Update(t.Context(), "used", headless)
	require.NoError(t, err)
	requireEvents(t, ch, 1)
}

func TestActionsServicePublishesOnEveryMutatingMethod(t *testing.T) {
	actionStore, _ := newServiceStore(t)
	service, ch := newTestActionsService(t, actionStore)
	ctx := t.Context()

	_, err := service.Create(ctx, serviceAction("run"))
	require.NoError(t, err)
	assert.Equal(t, len(actionStore.List()), requireEvents(t, ch, 1)[0].Count, "Create")

	_, err = service.Update(ctx, "run", serviceAction("run"))
	require.NoError(t, err)
	assert.Equal(t, len(actionStore.List()), requireEvents(t, ch, 1)[0].Count, "Update")

	require.NoError(t, service.Reorder(ctx, []string{"run"}))
	assert.Equal(t, len(actionStore.List()), requireEvents(t, ch, 1)[0].Count, "Reorder")

	launcher, err := service.CreateLauncher(ctx, actions.Launcher{ID: "launch", Label: "Launch", Command: "true"})
	require.NoError(t, err)
	assert.Equal(t, len(actionStore.List()), requireEvents(t, ch, 1)[0].Count, "CreateLauncher")

	_, err = service.UpdateLauncher(ctx, "launch", launcher)
	require.NoError(t, err)
	assert.Equal(t, len(actionStore.List()), requireEvents(t, ch, 1)[0].Count, "UpdateLauncher")

	require.NoError(t, service.DeleteLauncher(ctx, "launch"))
	assert.Equal(t, len(actionStore.List()), requireEvents(t, ch, 1)[0].Count, "DeleteLauncher")

	require.NoError(t, service.Delete(ctx, "run"))
	assert.Equal(t, len(actionStore.List()), requireEvents(t, ch, 1)[0].Count, "Delete")
}

// flowOnlyUsage is the half of the usage check this adapter test cares about.
// The full checker, including the nonterminal command count, is core logic
// and is tested in cmd/desktop/internal/app.
type flowOnlyUsage struct{ flows *flow.FlowStore }

func (u flowOnlyUsage) Usage(_ context.Context, id string) (actions.ActionUsage, error) {
	usage := actions.ActionUsage{}
	for _, f := range u.flows.List() {
		for _, n := range f.Nodes {
			if cfg, ok := n.Config.(*flow.ActionConfig); ok && cfg.Action == id {
				usage.FlowIDs = append(usage.FlowIDs, f.ID)
				break
			}
		}
	}
	return usage, nil
}
