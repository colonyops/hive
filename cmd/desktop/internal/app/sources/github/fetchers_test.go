package github_test

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// connectorNode is the flow node a fixture instance is built for.
func connectorNode() connector.Node {
	return connector.Node{FlowID: "triage", NodeID: "in-prs"}
}

func newFetchers() *ghsource.Fetchers {
	return ghsource.NewFetchers(ghclient.NewClient(), credentials.NewMemoryStore(), zerolog.Nop())
}

// One fetcher per account, and the same one every time for the same account.
//
// A LiveProvider holds one account's response cache, its conditional-request
// state, and its rate-limit cooldown. Handing two accounts the same provider
// would serve one account's items to the other and let one account's rate
// limit stall every other; handing one account a fresh provider per node would
// throw away the cache that makes two nodes on the same query cost one
// request.
func TestFetchersAreOnePerAccount(t *testing.T) {
	t.Parallel()

	fetchers := newFetchers()
	octocat := credentials.Ref{Provider: ghsource.Provider, Account: "octocat"}
	hubot := credentials.Ref{Provider: ghsource.Provider, Account: "hubot"}

	first, again := fetchers.For(octocat), fetchers.For(octocat)
	assert.Same(t, first, again,
		"the same account must reuse its fetcher, or its cache is thrown away")
	assert.NotSame(t, first, fetchers.For(hubot),
		"two accounts sharing a fetcher would share a cache and a rate limit")
}

// A node names its account, and the factory resolves it at construction. A
// config naming another provider's credential is a mistake worth failing on:
// letting it through would surface as an empty feed rather than an error.
func TestFactoryRejectsACredentialFromAnotherProvider(t *testing.T) {
	t.Parallel()

	factory := ghsource.NewFactory(newFetchers())
	node := connectorNode()

	for name, credential := range map[string]string{
		"another provider": "grafana/prod",
		"not a ref":        "octocat",
		"empty":            "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := factory.New(node, &ghsource.Config{
				Credential: credential,
				Kind:       ghsource.KindSearch,
				Query:      "is:open",
			})
			require.Errorf(t, err, "credential %q was accepted", credential)
		})
	}
}

// The account scopes the instance's ingestion metadata, which is what keeps
// two accounts' items distinguishable inside one flow.
func TestInstanceScopesItemsToItsAccount(t *testing.T) {
	t.Parallel()

	factory := ghsource.NewFactory(newFetchers())
	instance, err := factory.New(connectorNode(), &ghsource.Config{
		Credential: ghsource.Provider + "/octocat",
		Kind:       ghsource.KindNotifications,
	})
	require.NoError(t, err)
	assert.Equal(t, "octocat", instance.Metadata.SourceScope)
}
