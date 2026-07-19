package main

import (
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

// actionsRefs resolves a flow's only remaining cross-file reference — the
// action node's actions.yml action id — against the actions store,
// satisfying flow.Refs. Source and feed nodes are now self-contained (a
// source embeds its fetch config, a feed's identity is its node id), so this
// adapter no longer touches the feed provider.
type actionsRefs struct {
	actions *actions.ActionStore
}

func newActionsRefs(actionStore *actions.ActionStore) *actionsRefs {
	return &actionsRefs{actions: actionStore}
}

func (a *actionsRefs) ResolveAction(id string) bool {
	_, ok := a.actions.Get(id)
	return ok
}
