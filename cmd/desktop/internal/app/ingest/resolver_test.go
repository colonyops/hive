package ingest

import (
	"context"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	whsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

type fakeFlows []flow.Flow

func (f fakeFlows) List() []flow.Flow { return f }

// stubFactories builds instances without any connector's real dependencies.
// The resolver's job is which nodes become instances, not what an instance
// then does, so the factories only need to record what they were handed.
func stubFactories() map[string]connector.Factory {
	build := func(connectorType string) connector.Factory {
		return connector.Factory{
			New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
				return connector.Instance{Type: connectorType, Node: node, Config: cfg}, nil
			},
		}
	}
	return map[string]connector.Factory{
		ghsource.Descriptor.Type: build(ghsource.Descriptor.Type),
		whsource.Descriptor.Type: build(whsource.Descriptor.Type),
	}
}

func githubNode(id, kind, query string) flow.Node {
	return flow.Node{
		ID:     id,
		Type:   ghsource.Descriptor.Type,
		Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: "github/octocat", Kind: kind, Query: query}),
	}
}

func webhookNode(id, path string) flow.Node {
	return flow.Node{
		ID:     id,
		Type:   whsource.Descriptor.Type,
		Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: path}),
	}
}

func newTestResolver(flows fakeFlows) *Resolver {
	return NewResolver(flows, stubFactories(), zerolog.Nop())
}

// Enablement is the resolver's alone now: neither the producer nor the
// webhook listener filters, so a disabled flow or node that still resolved
// would poll GitHub and accept deliveries for something the user switched off.
func TestResolverSkipsDisabledFlowsAndNodes(t *testing.T) {
	t.Parallel()

	disabled := githubNode("off", "notifications", "")
	disabled.Disabled = true

	resolver := newTestResolver(fakeFlows{
		{
			ID:      "triage",
			Enabled: true,
			Nodes: []flow.Node{
				githubNode("live", "search", "is:open"),
				disabled,
				{ID: "sink", Type: "feed", Config: &flow.FeedConfig{}},
			},
		},
		{
			ID:      "paused",
			Enabled: false,
			Nodes:   []flow.Node{githubNode("in", "notifications", "")},
		},
	})

	instances := resolver.PullInstances(t.Context())
	require.Len(t, instances, 1)
	assert.Equal(t, "triage/live", instances[0].Node.ID())
}

// Pull and push are resolved separately so neither ingress ever sees the
// other's instances: handing a push instance to the producer would have it
// dereference a nil PullSource on the next tick.
func TestResolverSeparatesPullFromPush(t *testing.T) {
	t.Parallel()

	resolver := newTestResolver(fakeFlows{{
		ID:      "triage",
		Enabled: true,
		Nodes: []flow.Node{
			githubNode("poll", "search", "is:open"),
			webhookNode("hook", "ci-alerts"),
		},
	}})

	pull := resolver.PullInstances(t.Context())
	require.Len(t, pull, 1)
	assert.Equal(t, "triage/poll", pull[0].Node.ID())

	push := resolver.PushInstances(t.Context())
	require.Len(t, push, 1)
	assert.Equal(t, "triage/hook", push[0].Node.ID())
}

// A connector with no factory is the mock-mode case: GitHub has no fetcher to
// construct instances over. The node must be skipped rather than resolving to
// a half-built instance the producer would then drain.
func TestResolverSkipsConnectorsWithoutAFactory(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(
		fakeFlows{{
			ID:      "triage",
			Enabled: true,
			Nodes: []flow.Node{
				githubNode("poll", "search", "is:open"),
				webhookNode("hook", "ci-alerts"),
			},
		}},
		map[string]connector.Factory{whsource.Descriptor.Type: stubFactories()[whsource.Descriptor.Type]},
		zerolog.Nop(),
	)

	assert.Empty(t, resolver.PullInstances(t.Context()), "a connector with no factory must not resolve")
	assert.Len(t, resolver.PushInstances(t.Context()), 1)
}

// One node's factory failing must not cost the tick every other source.
func TestResolverSkipsInstancesThatFailToBuild(t *testing.T) {
	t.Parallel()

	factories := stubFactories()
	factories[ghsource.Descriptor.Type] = connector.Factory{
		New: func(node connector.Node, _ connector.Config) (connector.Instance, error) {
			if node.NodeID == "broken" {
				return connector.Instance{}, fmt.Errorf("cannot build")
			}
			return connector.Instance{Type: ghsource.Descriptor.Type, Node: node}, nil
		},
	}

	resolver := NewResolver(fakeFlows{{
		ID:      "triage",
		Enabled: true,
		Nodes: []flow.Node{
			githubNode("broken", "search", "is:open"),
			githubNode("healthy", "search", "is:open"),
		},
	}}, factories, zerolog.Nop())

	instances := resolver.PullInstances(t.Context())
	require.Len(t, instances, 1)
	assert.Equal(t, "triage/healthy", instances[0].Node.ID())
}

// The resurface policy is the flow's, and it reaches the connector through
// the node rather than being read from config: a source cannot opt out of
// what its flow declared.
func TestResolverCarriesTheFlowResurfacePolicy(t *testing.T) {
	t.Parallel()

	resolver := newTestResolver(fakeFlows{{
		ID:        "triage",
		Enabled:   true,
		Resurface: flow.ResurfacePolicy("never"),
		Nodes:     []flow.Node{githubNode("poll", "search", "is:open")},
	}})

	instances := resolver.PullInstances(t.Context())
	require.Len(t, instances, 1)
	assert.EqualValues(t, "never", instances[0].Node.Policy)
}

// Prefetch is dispatched per connector type, and only for the types that
// wired it — a connector without a batched pre-pass must not be called with
// another connector's instances.
func TestResolverPrefetchesPerConnectorType(t *testing.T) {
	t.Parallel()

	var batched []connector.Instance
	factories := stubFactories()
	gh := factories[ghsource.Descriptor.Type]
	gh.Prefetch = func(_ context.Context, instances []connector.Instance) error {
		batched = instances
		return nil
	}
	factories[ghsource.Descriptor.Type] = gh

	resolver := NewResolver(fakeFlows{{
		ID:      "triage",
		Enabled: true,
		Nodes: []flow.Node{
			githubNode("a", "search", "is:open"),
			githubNode("b", "search", "is:closed"),
			webhookNode("hook", "ci-alerts"),
		},
	}}, factories, zerolog.Nop())

	all := append(resolver.PullInstances(t.Context()), resolver.PushInstances(t.Context())...)
	require.NoError(t, resolver.Prefetch(t.Context(), all))

	require.Len(t, batched, 2, "both GitHub sources should be offered in one batch")
	for _, instance := range batched {
		assert.Equal(t, ghsource.Descriptor.Type, instance.Type)
	}
}
