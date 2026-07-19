package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/feed"
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
// sources, profiles feeds) against the desktop feed provider, satisfying
// flow.Refs. It is built on feed.Provider — not the concrete *feed.Store —
// so it works uniformly across live and mock feed backends alike; the
// store's on-disk config is exactly what Provider.Sources/Profiles already
// expose.
//
// Actions are not wired yet: Phase 5 supplies the real .hive/actions.yml
// action set. Until then ResolveAction always reports unresolved, so any
// flow with an action node fails validation visibly rather than silently
// passing.
type flowRefsAdapter struct {
	provider feed.Provider
}

func newFlowRefsAdapter(provider feed.Provider) *flowRefsAdapter {
	return &flowRefsAdapter{provider: provider}
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

func (a *flowRefsAdapter) ResolveAction(string) bool {
	return false
}
