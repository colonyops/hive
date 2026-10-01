package app

import (
	"context"
	"sort"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// Integration is one card on the Integrations screen: a projection of the
// registry, not a second list — an unregistered connector is not an
// integration. Cards are grouped by credentials provider, so a provider that
// ships several connector types (Grafana's metrics and alerts) shows as one
// card; a connector with no provider (the webhook listener) is keyed by its
// type.
type Integration struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Stability string `json:"stability"`
	// Provider is empty when the connector needs no credential, which also means
	// the account fields below are meaningless rather than merely empty.
	Provider string   `json:"provider"`
	Types    []string `json:"types"`
	// Accounts is every connected account for Provider, sorted. Names only — a
	// value never leaves the credential store.
	Accounts []string `json:"accounts"`
	// EnvOverride reports this provider's environment override is set, which
	// authenticates every fetch without a stored account. Without it a headless
	// or CI run would show "Not connected" beside a working feed.
	EnvOverride bool `json:"envOverride"`
}

func (i Integration) Connected() bool { return len(i.Accounts) > 0 || i.EnvOverride }

// IntegrationsService lists the connector registry with each entry's connection
// state. It is deliberately generic — acquiring a credential is provider-specific
// and lives on that connector's own service (GitHubService's device flow), so
// adding a connector is a change to cmd/desktop/internal/app/sources alone.
type IntegrationsService struct{ creds credentials.Store }

func newIntegrationsService(creds credentials.Store) *IntegrationsService {
	return &IntegrationsService{creds: creds}
}

// List returns one card per connector family, sorted by key so the screen's
// order is stable — Go map iteration is randomized.
func (s *IntegrationsService) List(context.Context) ([]Integration, error) {
	// Group by card key — the provider, else the type — so a family like Grafana
	// shows one card rather than one per node type.
	groups := map[string][]connector.Descriptor{}
	for _, d := range sources.All() {
		key := d.Provider
		if key == "" {
			key = d.Type
		}
		groups[key] = append(groups[key], d)
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]Integration, 0, len(keys))
	for _, key := range keys {
		ds := groups[key]
		sort.Slice(ds, func(i, j int) bool { return ds[i].Type < ds[j].Type })

		types := make([]string, len(ds))
		for i, d := range ds {
			types[i] = d.Type
		}

		integration := Integration{
			Key:       key,
			Title:     groupTitle(ds),
			Stability: leastStable(ds).String(),
			Provider:  ds[0].Provider,
			Types:     types,
			// A nil slice marshals to null; the frontend reads Accounts unguarded.
			Accounts: []string{},
		}

		if provider := ds[0].Provider; provider != "" {
			refs, err := credentials.ListProvider(s.creds, provider)
			if err != nil {
				return nil, Wrap(err, KindInternal, "listing connected accounts")
			}
			for _, ref := range refs {
				integration.Accounts = append(integration.Accounts, ref.Account)
			}
			integration.EnvOverride = credentials.HasEnvOverride(provider)
		}

		out = append(out, integration)
	}
	return out, nil
}

// groupTitle titles a card. A multi-type provider declares a ProviderTitle so
// its card is named once ("Grafana"); deriving the title from the descriptors'
// own titles would couple the card label to wording that can be reworded freely.
func groupTitle(ds []connector.Descriptor) string {
	for _, d := range ds {
		if d.ProviderTitle != "" {
			return d.ProviderTitle
		}
	}
	return ds[0].Title
}

// leastStable is the most conservative stability across a family, so a card
// with any experimental node type does not read "stable".
func leastStable(ds []connector.Descriptor) connector.Stability {
	least := ds[0].Stability
	for _, d := range ds[1:] {
		if d.Stability < least {
			least = d.Stability
		}
	}
	return least
}
