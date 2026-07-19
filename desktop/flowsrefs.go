package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
)

// flowSourceKinds maps a profiles source's Kind ("search"/"notifications")
// to the flow schema's github-* source kind naming (see
// flow.GithubSourceConfig.Validate). A kind with no mapping (e.g. a future
// "rpc" source kind) passes through unchanged.
var flowSourceKinds = map[string]string{
	"search":        "github-search",
	"notifications": "github-notifications",
}

// flowRefsAdapter resolves a flow's cross-file references (profiles
// sources, profiles feeds, actions.yml actions) against the desktop feed
// provider and the actions store, satisfying flow.Refs. It is built on
// feed.Provider — not the concrete *feed.Store — so it works uniformly
// across live and mock feed backends alike; the store's on-disk config is
// exactly what Provider.Sources/Profiles already expose.
type flowRefsAdapter struct {
	provider feed.Provider
	actions  *actions.ActionStore
}

func newFlowRefsAdapter(provider feed.Provider, actionStore *actions.ActionStore) *flowRefsAdapter {
	return &flowRefsAdapter{provider: provider, actions: actionStore}
}

func (a *flowRefsAdapter) ResolveSource(id string) (string, bool) {
	sources, err := a.provider.Sources(context.Background())
	if err != nil {
		return "", false
	}
	for _, src := range sources {
		if src.ID != id {
			continue
		}
		if kind, ok := flowSourceKinds[src.Kind]; ok {
			return kind, true
		}
		return src.Kind, true
	}
	return "", false
}

func (a *flowRefsAdapter) ResolveFeed(id string) bool {
	profiles, err := a.provider.Profiles(context.Background())
	if err != nil {
		return false
	}
	for _, p := range profiles {
		for _, f := range p.Feeds {
			if f.ID == id {
				return true
			}
		}
	}
	return false
}

func (a *flowRefsAdapter) ResolveAction(id string) bool {
	_, ok := a.actions.Get(id)
	return ok
}
