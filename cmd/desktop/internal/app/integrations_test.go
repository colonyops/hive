package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	grafana "github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana"
	"github.com/colonyops/hive/internal/platform/credentials"
)

func integrationsFor(t *testing.T, creds credentials.Store) map[string]Integration {
	t.Helper()
	list, err := newIntegrationsService(creds).List(t.Context())
	require.NoError(t, err)

	byKey := make(map[string]Integration, len(list))
	for _, integration := range list {
		byKey[integration.Key] = integration
	}
	return byKey
}

// The screen is a projection of the registry, not a second list beside it. It
// used to be a hardcoded array, which is a list that drifts silently: a
// connector added to the registry simply never got a card. Cards are grouped
// by provider, so the projection covers every descriptor without one card per
// node type.
func TestIntegrationsCoverEveryRegisteredConnector(t *testing.T) {
	t.Parallel()

	list, err := newIntegrationsService(credentials.NewMemoryStore()).List(t.Context())
	require.NoError(t, err)

	covered := map[string]bool{}
	for _, integration := range list {
		require.NotEmpty(t, integration.Types, "card %q covers no node types", integration.Key)
		for _, nodeType := range integration.Types {
			descriptor, ok := sources.Lookup(nodeType)
			require.Truef(t, ok, "card %q lists %q, which is not registered", integration.Key, nodeType)
			assert.Equal(t, descriptor.Provider, integration.Provider)
			assert.NotEqual(t, "unknown", integration.Stability, "card %q", integration.Key)
			covered[nodeType] = true
		}
	}
	assert.Len(t, covered, len(sources.All()), "every descriptor appears on exactly one card")
}

// A provider that ships several connector types shows once, titled by its
// declared ProviderTitle rather than once per node type — the grouping F12
// called for so alerts does not add a second Grafana card.
func TestIntegrationsGroupAProviderIntoOneCard(t *testing.T) {
	t.Parallel()

	grafanaCard, ok := integrationsFor(t, credentials.NewMemoryStore())[grafana.Provider]
	require.Truef(t, ok, "grafana is not grouped under one card keyed by its provider")
	assert.Equal(t, "Grafana", grafanaCard.Title, "the card title is the provider title, not one node type's title")
	assert.Contains(t, grafanaCard.Types, grafana.MetricsDescriptor.Type)
	assert.Contains(t, grafanaCard.Types, grafana.AlertsDescriptor.Type)
}

// leastStable is the card's stability, and a mixed-stability family must read as
// its least-stable member. No shipped family mixes stabilities, so a synthetic
// mix pins the discriminating case.
func TestLeastStableTakesTheMostConservative(t *testing.T) {
	t.Parallel()

	ds := []connector.Descriptor{
		{Stability: connector.Stable},
		{Stability: connector.Experimental},
	}
	assert.Equal(t, connector.Experimental, leastStable(ds))
	assert.Equal(t, connector.Experimental, leastStable([]connector.Descriptor{ds[1], ds[0]}), "order-independent")
}

// Go map iteration is randomized, so an unsorted projection would reshuffle the
// cards on every read — and the screen re-reads on every connection change.
func TestIntegrationsAreSortedByKey(t *testing.T) {
	t.Parallel()

	list, err := newIntegrationsService(credentials.NewMemoryStore()).List(t.Context())
	require.NoError(t, err)

	for i := 1; i < len(list); i++ {
		assert.Lessf(t, list[i-1].Key, list[i].Key, "entry %d is out of order", i)
	}
}

func TestIntegrationsReportConnectedAccounts(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	require.NoError(t, creds.Set(credentials.Ref{Provider: ghsource.Provider, Account: "octocat"}, "tok1"))
	require.NoError(t, creds.Set(credentials.Ref{Provider: ghsource.Provider, Account: "hubot"}, "tok2"))

	github := integrationsFor(t, creds)[ghsource.Provider]
	assert.Equal(t, []string{"hubot", "octocat"}, github.Accounts)
	assert.True(t, github.Connected())
}

// A connector with nothing to authenticate as is not "not connected" — the
// webhook listener is local ingress. Its card must not offer a Connect action,
// which is what an empty Provider tells the frontend.
func TestIntegrationWithoutAProviderReportsNoAccounts(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	require.NoError(t, creds.Set(credentials.Ref{Provider: ghsource.Provider, Account: "octocat"}, "tok1"))

	for key, integration := range integrationsFor(t, creds) {
		if integration.Provider != "" {
			continue
		}
		assert.Emptyf(t, integration.Accounts, "card %q has no provider but reports accounts", key)
		assert.Falsef(t, integration.EnvOverride, "card %q has no provider but reports an override", key)
	}
}

// Accounts is never nil: a nil slice marshals to null, and every frontend read
// of it would need a guard the type does not advertise.
func TestIntegrationAccountsAreNeverNil(t *testing.T) {
	t.Parallel()

	for key, integration := range integrationsFor(t, credentials.NewMemoryStore()) {
		assert.NotNilf(t, integration.Accounts, "card %q has nil accounts", key)
	}
}

// The environment override authenticates every fetch while naming no account,
// so a listing of stored refs is empty for it. Reporting that as disconnected
// would put "Not connected" beside a feed that is visibly working — which is
// exactly the state CI, the server build and the e2e harness all run in.
func TestIntegrationEnvOverrideCountsAsConnectedWithoutAStoredAccount(t *testing.T) {
	t.Setenv(credentials.EnvOverrideName(ghsource.Provider), "env-token")

	github := integrationsFor(t, credentials.NewMemoryStore())[ghsource.Provider]
	assert.Empty(t, github.Accounts)
	assert.True(t, github.EnvOverride)
	assert.True(t, github.Connected())
}

func TestIntegrationWithNoCredentialIsNotConnected(t *testing.T) {
	t.Parallel()

	github := integrationsFor(t, credentials.NewMemoryStore())[ghsource.Provider]
	assert.Empty(t, github.Accounts)
	assert.False(t, github.Connected())
}
