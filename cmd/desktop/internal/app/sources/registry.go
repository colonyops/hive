// Package sources is the source connector registry: the one place every
// connector type is declared, and the lookups the rest of the app resolves a
// connector through.
//
// It holds only declarations. A Descriptor is static data — type, title,
// mode, stability, capabilities, and a config factory — so this package can
// be imported by anything that needs to know a connector *exists*, including
// the flow package, which derives a node type from each entry. Constructing a
// running connector needs dependencies (the GitHub fetcher) and goes through
// connector.Factory, wired where those dependencies live.
//
// That split is also what keeps the import graph acyclic: this package
// imports the connector packages, so they must not import it. They name their
// vocabulary from internal/app/sources/connector instead.
package sources

import (
	"maps"
	"sort"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/exec"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/rss"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

// descriptors is the registry, keyed by the connector's namespaced type —
// which is also the flow node's `type:` discriminator. Adding a connector is
// one line here plus its Descriptor and implementation in its own package.
//
// Never init() self-registration: gochecknoinits is enabled, and an explicit
// map is the only form where the set of connectors can be read off one file.
var descriptors = map[string]connector.Descriptor{
	exec.Descriptor.Type:             exec.Descriptor,
	gitea.Descriptor.Type:            gitea.Descriptor,
	github.Descriptor.Type:           github.Descriptor,
	grafana.MetricsDescriptor.Type:   grafana.MetricsDescriptor,
	grafana.AlertsDescriptor.Type:    grafana.AlertsDescriptor,
	grafana.IRMAlertsDescriptor.Type: grafana.IRMAlertsDescriptor,
	posthog.ErrorsDescriptor.Type:    posthog.ErrorsDescriptor,
	posthog.AlertsDescriptor.Type:    posthog.AlertsDescriptor,
	rss.Descriptor.Type:              rss.Descriptor,
	webhook.Descriptor.Type:          webhook.Descriptor,
}

// Types returns every registered connector type in sorted order. Sorting
// keeps generated output — the node palette, the flows prompt — byte-stable
// across runs, since Go map iteration is randomized.
func Types() []string {
	types := make([]string, 0, len(descriptors))
	for name := range descriptors {
		types = append(types, name)
	}
	sort.Strings(types)
	return types
}

// Lookup returns the descriptor registered for a connector type.
func Lookup(connectorType string) (connector.Descriptor, bool) {
	d, ok := descriptors[connectorType]
	return d, ok
}

// All returns every descriptor, keyed by type. The returned map is a copy, so
// a caller ranging over it cannot mutate the registry.
func All() map[string]connector.Descriptor {
	out := make(map[string]connector.Descriptor, len(descriptors))
	maps.Copy(out, descriptors)
	return out
}
