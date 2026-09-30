package ingest

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// Resolver turns the current flow set into live connector instances. It is
// the one place the two halves of a connector's declaration meet: the
// registry says a node type is a source and which mode it ingests in, and the
// factories — wired where their dependencies live — construct the instance
// from the node's parsed config.
//
// It resolves on every call rather than caching, so a flow added, edited, or
// removed takes effect on the next tick or the next delivery, with no restart
// and no invalidation to get wrong.
type Resolver struct {
	flows     FlowLister
	factories map[string]connector.Factory
	logger    zerolog.Logger
}

// NewResolver builds a resolver over the live flow set and the factories for
// each registered connector type.
func NewResolver(flows FlowLister, factories map[string]connector.Factory, logger zerolog.Logger) *Resolver {
	return &Resolver{flows: flows, factories: factories, logger: logger}
}

// PullInstances is every enabled pull-mode instance: the sources a producer
// tick drains.
func (r *Resolver) PullInstances(ctx context.Context) []connector.Instance {
	return r.instances(ctx, connector.ModePull)
}

// PushInstances is every enabled push-mode instance: the sources an ingress
// resolves a delivery against.
func (r *Resolver) PushInstances(ctx context.Context) []connector.Instance {
	return r.instances(ctx, connector.ModePush)
}

// instances walks the enabled flows and constructs one instance per enabled
// source node of the requested mode. A node whose connector has no factory,
// or whose factory rejects its config, is logged and skipped: one
// misconfigured source must not take the rest of the tick down with it.
func (r *Resolver) instances(ctx context.Context, mode connector.Mode) []connector.Instance {
	var out []connector.Instance
	for _, f := range r.flows.List() {
		if !f.Enabled {
			continue
		}
		for _, node := range f.Nodes {
			if node.Disabled {
				continue
			}
			descriptor, ok := sources.Lookup(node.Type)
			if !ok || descriptor.Mode != mode {
				continue
			}
			instance, err := r.build(f, node)
			if err != nil {
				r.logger.Warn().Ctx(ctx).Err(err).Str("flow", f.ID).Str("node", node.ID).Msg("ingest: source unavailable")
				continue
			}
			out = append(out, instance)
		}
	}
	return out
}

// build constructs one node's instance through its connector's factory.
func (r *Resolver) build(f flow.Flow, node flow.Node) (connector.Instance, error) {
	config, ok := node.Config.(*flow.SourceConfig)
	if !ok {
		return connector.Instance{}, fmt.Errorf("node config is %T, want *flow.SourceConfig", node.Config)
	}
	factory, ok := r.factories[node.Type]
	if !ok || factory.New == nil {
		return connector.Instance{}, fmt.Errorf("connector %q has no factory", node.Type)
	}
	return factory.New(connector.Node{
		FlowID: f.ID,
		NodeID: node.ID,
		Policy: models.ResurfacePolicy(f.Resurface),
	}, config.Connector())
}

// Prefetch offers each connector type's instances to its batched pre-pass,
// for the types that declared CapBatchPrefetch. It runs before any source is
// drained so a provider that can answer many queries in one request does.
//
// A failure is advisory and reported, not returned: the individual sources
// still drain, just without the batch's head start.
func (r *Resolver) Prefetch(ctx context.Context, instances []connector.Instance) error {
	byType := make(map[string][]connector.Instance)
	for _, instance := range instances {
		byType[instance.Type] = append(byType[instance.Type], instance)
	}
	for connectorType, group := range byType {
		factory, ok := r.factories[connectorType]
		if !ok || factory.Prefetch == nil {
			continue
		}
		if err := factory.Prefetch(ctx, group); err != nil {
			return fmt.Errorf("connector %q: %w", connectorType, err)
		}
	}
	return nil
}
