// Package rss is the feed source connector: it fetches one RSS, Atom, or JSON
// Feed URL on the poll tick and ingests its entries as items, so a blog, a
// release feed, or a status page is a first-party source with no bridge in
// between.
//
// It is the one pull connector whose snapshot is a *window* rather than the
// complete set. A feed drops old entries as new ones arrive, so an entry that
// stopped appearing is old, not resolved: the connector declares neither
// CapClassify nor CapConfirmAbsence, and an entry that scrolls off the end is
// left to the flow's resurface policy (ADR an-rss-window-is-not-an-authoritative-set).
package rss

import (
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// SourceKind is the inbox source_kind a feed observation carries.
const SourceKind = "rss"

// ItemKind is the canonical `kind` every feed entry carries, so
// `applies_to: [Post]` targets one whichever feed it came from.
const ItemKind = "Post"

// Descriptor declares the connector. No Provider: it reads public feeds only,
// so there is no account for the app to hold or connect.
var Descriptor = connector.Descriptor{
	Type:      "sources.rss",
	Title:     "RSS feed",
	Mode:      connector.ModePull,
	Stability: connector.Stable,
	NewConfig: func() connector.Config { return &Config{} },
}

// NewFactory builds the instance half of the declaration over the shared
// feed client.
func NewFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*Config)
			if !ok {
				return connector.Instance{}, fmt.Errorf("rss source %q: config is %T, want *rss.Config", node.ID(), cfg)
			}
			return connector.Instance{
				Type: Descriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:  node.FlowID,
					SourceKind: SourceKind,
					// Several feeds can share a flow, so the node id is what
					// separates their inbox rows — as for webhook and exec.
					SourceScope: node.NodeID,
					Policy:      node.Policy,
				},
				Pull: &source{
					id:      node.ID(),
					topic:   node.Topic(),
					url:     config.URL,
					limit:   config.EntryLimit(),
					fetcher: fetchers,
				},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
	}
}
