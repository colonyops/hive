// Package exec is the command source connector: it runs the user's own
// command on the poll tick and ingests its stdout as an authoritative snapshot,
// so a CLI is a first-party source with no external scheduler, no webhook
// bridge, and no hand-rolled state file.
//
// It is the pull counterpart to the webhook connector's push, and it carries
// the property a webhook structurally cannot: stdout is the complete current
// set, so an item that stops appearing is authoritatively gone. See ADR a-command-is-a-source
// for the trust, failure and cadence decisions the shape rests on.
package exec

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// SourceKind is the inbox source_kind an exec observation carries.
const SourceKind = "exec"

// Environment supplies the environment the command runs in. A desktop launch
// inherits PATH=/usr/bin:/bin:/usr/sbin:/sbin, and the commands worth polling
// live wherever a package manager put them (ADR subprocess-environment).
type Environment interface {
	Environ(ctx context.Context) []string
}

// Descriptor declares the connector. It is pull: the producer drains it on a
// tick, and the config's own interval is a floor under that.
//
// It declares CapClassify and CapConfirmAbsence for the same reason
// sources.grafana_alerts does: stdout is the complete current set by contract,
// so an item that left it is resolved rather than merely unseen.
var Descriptor = connector.Descriptor{
	Type:  "sources.exec",
	Title: "Command source",
	// No Provider: the command authenticates itself with the user's own
	// config, so there is no account for the app to hold or connect.
	Mode:         connector.ModePull,
	Stability:    connector.Stable,
	Capabilities: connector.CapClassify | connector.CapConfirmAbsence,
	NewConfig:    func() connector.Config { return &Config{} },
}

// NewFactory builds the instance half of the declaration over the resolved
// subprocess environment.
func NewFactory(env Environment) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*Config)
			if !ok {
				return connector.Instance{}, fmt.Errorf("exec source %q: config is %T, want *exec.Config", node.ID(), cfg)
			}
			return connector.Instance{
				Type: Descriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:  node.FlowID,
					SourceKind: SourceKind,
					// Several exec sources can share a flow, so the node id is
					// what separates their inbox rows — as for webhook.
					SourceScope: node.NodeID,
					Policy:      node.Policy,
				},
				Pull: &source{
					id:      node.ID(),
					topic:   node.Topic(),
					command: config.Command,
					cwd:     config.Cwd,
					env:     config.Env,
					timeout: config.Timeout.Duration(),
					environ: env,
				},
				MinInterval: config.Interval.Duration(),
				Classifier:  canonical.Classifier{},
				Absence:     absence{},
				Config:      config,
			}, nil
		},
	}
}
