// Package relay is the Hive Relay source connector. The relay stores any HTTP
// request sent to a source's ingress URL and parses nothing; this connector
// pulls those deliveries by cursor, parses them with relayformat, applies the
// node's map, and emits the resulting items.
package relay

import (
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

const (
	Provider   = "relay"
	SourceKind = "relay"
)

// Descriptor declares the connector. CapClassify makes a successful Produce an
// authoritative snapshot, which is why Produce emits the whole cached set on
// every tick. There is no CapConfirmAbsence: the set is the relay's retention
// window, so an item that left it is old, not resolved.
var Descriptor = connector.Descriptor{
	Type:         "sources.relay",
	Title:        "Hive Relay",
	Provider:     Provider,
	Mode:         connector.ModePull,
	Stability:    connector.Experimental,
	Capabilities: connector.CapClassify,
	NewConfig:    func() connector.Config { return &Config{} },
}

func NewFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*Config)
			if !ok {
				return connector.Instance{}, fmt.Errorf("relay source %q: config is %T, want *relay.Config", node.ID(), cfg)
			}
			ref, err := config.CredentialRef()
			if err != nil {
				return connector.Instance{}, fmt.Errorf("relay source %q: %w", node.ID(), err)
			}
			return connector.Instance{
				Type: Descriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:   node.FlowID,
					SourceKind:  SourceKind,
					SourceScope: ref.Account,
					Policy:      node.Policy,
				},
				Pull: &source{
					id:       node.ID(),
					topic:    node.Topic(),
					cfg:      config,
					ref:      ref,
					fetchers: fetchers,
				},
				Classifier:  canonical.Classifier{},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
	}
}
