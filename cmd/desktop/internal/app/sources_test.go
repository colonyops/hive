package app

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	execsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/exec"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	ghclient "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/rss"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/internal/platform/execenv"
)

// testFetchers builds the per-account fetcher registry without touching the
// network or a keychain. Only its identity matters here — the factories hold
// it, they do not fetch through it.
func testFetchers() *ghsource.Fetchers {
	return ghsource.NewFetchers(ghclient.NewClient(), credentials.NewMemoryStore(), zerolog.Nop())
}

// testGrafanaFetchers is the same for the Grafana connector: a real registry
// over an empty stack store and memory credentials, never fetched through here.
func testGrafanaFetchers(t *testing.T) *grafana.Fetchers {
	t.Helper()
	stacks := grafana.NewStackStore(filepath.Join(t.TempDir(), "grafana-stacks.json"))
	return grafana.NewFetchers(stacks, credentials.NewMemoryStore(), zerolog.Nop())
}

// testPostHogFetchers is the same for the PostHog connector.
func testPostHogFetchers(t *testing.T) *posthog.Fetchers {
	t.Helper()
	projects := posthog.NewProjectStore(filepath.Join(t.TempDir(), "posthog-projects.json"))
	return posthog.NewFetchers(projects, credentials.NewMemoryStore(), zerolog.Nop())
}

// testGiteaFetchers is the same for the Gitea connector.
func testGiteaFetchers(t *testing.T) *gitea.Fetchers {
	t.Helper()
	instances := gitea.NewInstanceStore(filepath.Join(t.TempDir(), "gitea-instances.json"))
	return gitea.NewFetchers(instances, credentials.NewMemoryStore(), zerolog.Nop())
}

// A connector's declaration is in two halves: the descriptor says what it is
// and what it supports, and the factory is what actually constructs it. They
// are separate because only the factory needs dependencies — but that is also
// how they can disagree. A descriptor with no factory is a source node the
// editor offers and nothing ever polls, which surfaces as "my feed is empty"
// rather than as a failure.
func TestFactoriesCoverEveryDescriptor(t *testing.T) {
	t.Parallel()

	factories := sourceFactories(testFetchers(), testGrafanaFetchers(t), testPostHogFetchers(t), testGiteaFetchers(t), rss.NewFetchers(zerolog.Nop()), execenv.NewResolver(execenv.Options{}))

	for _, connectorType := range sources.Types() {
		factory, ok := factories[connectorType]
		require.Truef(t, ok, "connector %q is registered but has no factory", connectorType)
		assert.NotNilf(t, factory.New, "connector %q has a factory that constructs nothing", connectorType)
	}

	for connectorType := range factories {
		_, ok := sources.Lookup(connectorType)
		assert.Truef(t, ok, "a factory is wired for %q, which is not a registered connector", connectorType)
	}
}

// This is the "declared, not sniffed" rule made mechanical. The producer
// reads a capability off the instance instead of type-asserting for it, which
// only helps if what a descriptor promises is what its factory delivers: a
// declared-but-unwired capability is silently no classification, no absence
// confirmation, or no batched prefetch.
func TestFactoriesMatchDescribedCapabilities(t *testing.T) {
	t.Parallel()

	factories := sourceFactories(testFetchers(), testGrafanaFetchers(t), testPostHogFetchers(t), testGiteaFetchers(t), rss.NewFetchers(zerolog.Nop()), execenv.NewResolver(execenv.Options{}))

	for _, connectorType := range sources.Types() {
		descriptor, _ := sources.Lookup(connectorType)
		factory := factories[connectorType]

		assert.Equalf(t, descriptor.Capabilities.Has(connector.CapBatchPrefetch), factory.Prefetch != nil,
			"connector %q declares CapBatchPrefetch=%v but wires Prefetch=%v",
			connectorType, descriptor.Capabilities.Has(connector.CapBatchPrefetch), factory.Prefetch != nil)

		// The per-instance capabilities can only be checked on a constructed
		// instance, so build one from a config the connector accepts.
		config := descriptor.NewConfig()
		require.NoErrorf(t, seedValidConfig(config), "connector %q", connectorType)
		require.NoErrorf(t, config.Validate(), "connector %q: the seeded config is not valid", connectorType)

		instance, err := factory.New(connector.Node{FlowID: "f", NodeID: "n"}, config)
		require.NoErrorf(t, err, "connector %q", connectorType)

		assert.Equalf(t, descriptor.Capabilities.Has(connector.CapClassify), instance.Classifier != nil,
			"connector %q declares CapClassify=%v but sets Classifier=%v",
			connectorType, descriptor.Capabilities.Has(connector.CapClassify), instance.Classifier != nil)
		assert.Equalf(t, descriptor.Capabilities.Has(connector.CapConfirmAbsence), instance.Absence != nil,
			"connector %q declares CapConfirmAbsence=%v but sets Absence=%v",
			connectorType, descriptor.Capabilities.Has(connector.CapConfirmAbsence), instance.Absence != nil)

		// Pull and push are distinct: exactly the pull-mode connectors carry
		// something to drain, and a push connector must not be handed to the
		// producer with a nil Pull for it to dereference.
		assert.Equalf(t, descriptor.Mode == connector.ModePull, instance.Pull != nil,
			"connector %q is mode %v but sets Pull=%v", connectorType, descriptor.Mode, instance.Pull != nil)

		assert.Equalf(t, connectorType, instance.Type, "connector %q builds an instance of another type", connectorType)
		assert.NotEmptyf(t, instance.Metadata.SourceKind, "connector %q builds an instance with no source kind", connectorType)
		assert.Equalf(t, "f", instance.Metadata.ProfileID, "connector %q does not scope items to the owning flow", connectorType)
	}
}

// A pull connector's `interval` is a promise the factory has to keep. The
// producer reads the floor off the instance, so a config field that no factory
// copies into MinInterval is a setting the editor offers and the poll loop
// ignores — the source keeps running every tick and the user has no way to
// tell.
//
// Reflection rather than a per-connector case: the point is that a connector
// added later cannot quietly skip the wiring.
func TestPullFactoriesCarryTheConfiguredInterval(t *testing.T) {
	t.Parallel()

	factories := sourceFactories(testFetchers(), testGrafanaFetchers(t), testPostHogFetchers(t), testGiteaFetchers(t), rss.NewFetchers(zerolog.Nop()), execenv.NewResolver(execenv.Options{}))
	const floor = 90 * time.Minute

	for _, connectorType := range sources.Types() {
		descriptor, _ := sources.Lookup(connectorType)
		if descriptor.Mode != connector.ModePull {
			continue
		}

		config := descriptor.NewConfig()
		require.NoErrorf(t, seedValidConfig(config), "connector %q", connectorType)
		interval := reflect.ValueOf(config).Elem().FieldByName("Interval")
		require.Truef(t, interval.IsValid() && interval.CanSet(), "pull connector %q has no settable Interval field", connectorType)
		interval.Set(reflect.ValueOf(connector.Duration(floor)))
		require.NoErrorf(t, config.Validate(), "connector %q rejects an interval", connectorType)

		instance, err := factories[connectorType].New(connector.Node{FlowID: "f", NodeID: "n"}, config)
		require.NoErrorf(t, err, "connector %q", connectorType)
		assert.Equalf(t, floor, instance.MinInterval,
			"connector %q accepts an interval in its config but does not declare it as MinInterval", connectorType)
	}
}

// Mock modes construct no fetchers. The GitHub connector must then be absent
// from the factory map rather than present with a nil registry, because the
// resolver skips a connector with no factory and would dereference one that
// has a broken factory.
func TestGithubFactoryIsAbsentWithoutAFetcher(t *testing.T) {
	t.Parallel()

	factories := sourceFactories(nil, nil, nil, nil, nil, execenv.NewResolver(execenv.Options{}))

	_, ok := factories[ghsource.Descriptor.Type]
	assert.False(t, ok, "the GitHub connector is wired without a fetcher to construct it over")
	_, ok = factories[webhook.Descriptor.Type]
	assert.True(t, ok, "the webhook connector needs no fetcher and must stay wired in mock modes")
}

// seedValidConfig fills a connector's config with the minimum its Validate
// accepts. It switches on the concrete type rather than reflecting over
// fields so that adding a connector fails here loudly and once: a generic
// filler would quietly produce something Validate rejects, and the failure
// would read as a broken factory rather than a missing case.
func seedValidConfig(config connector.Config) error {
	switch c := config.(type) {
	case *ghsource.Config:
		c.Credential = ghsource.Provider + "/octocat"
		c.Kind, c.Query = ghsource.KindSearch, "is:open is:pr"
	case *webhook.Config:
		c.Path = "ci-alerts"
	case *execsource.Config:
		c.Command, c.Timeout = "echo '[]'", connector.Duration(30*time.Second)
	case *grafana.MetricsConfig:
		c.Credential = grafana.Provider + "/grafana.example.com-1"
		c.DatasourceUID, c.Expr = "prometheus-uid", "up"
	case *grafana.AlertsConfig:
		c.Credential = grafana.Provider + "/grafana.example.com-1"
	case *grafana.IRMAlertsConfig:
		c.Credential = grafana.Provider + "/grafana.example.com-1"
	case *posthog.ErrorsConfig:
		c.Credential = posthog.Provider + "/us.posthog.com-1"
	case *posthog.AlertsConfig:
		c.Credential = posthog.Provider + "/us.posthog.com-1"
	case *gitea.Config:
		c.Credential = gitea.Provider + "/gitea.example.com-octocat"
		c.Kind = gitea.KindSearch
	case *rss.Config:
		c.URL = "https://example.com/feed.xml"
	default:
		return fmt.Errorf("no valid config seed for %T; add one alongside the connector", config)
	}
	return nil
}
