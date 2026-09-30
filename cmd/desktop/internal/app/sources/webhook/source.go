// Package webhook is the webhook source connector: the config a webhook
// source node carries, the descriptor declaring what the connector supports,
// and the local HTTP listener deliveries arrive on.
//
// It is the push counterpart to the GitHub connector's poll. Nothing here
// implements connector.PullSource: a delivery is ingested when it arrives
// rather than when a tick asks for it.
package webhook

import (
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// NewFactory builds the instance half of the declaration. A webhook instance
// needs no outbound dependency — the listener owns ingress, and the instance
// exists so the listener can resolve which nodes serve a path and with what
// classification.
func NewFactory() connector.Factory {
	events := canonical.Classifier{}

	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*Config)
			if !ok {
				return connector.Instance{}, fmt.Errorf("webhook source %q: config is %T, want *webhook.Config", node.ID(), cfg)
			}
			return connector.Instance{
				Type: Descriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:  node.FlowID,
					SourceKind: SourceKind,
					// Several webhook sources can share a flow, so the node id
					// is what separates their inbox rows.
					SourceScope: node.NodeID,
					Policy:      node.Policy,
				},
				Classifier: events,
				Config:     config,
			}, nil
		},
	}
}
