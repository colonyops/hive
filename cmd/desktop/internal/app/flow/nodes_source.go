package flow

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// SourceConfig is the node-graph half of a source connector: it adapts a
// connector.Config — which knows only its own fields and how to validate them
// — to the NodeConfig every node type implements.
//
// The port counts live here rather than on the connector because they are
// graph vocabulary: a source is 0-in/1-out by definition, and a connector
// that had to declare it could get it wrong. Validate forwards, because a
// source node's config is self-contained; Refs resolves the action node's
// cross-file reference and a source has none.
type SourceConfig struct {
	connectorType string
	inner         connector.Config
}

// NewSourceConfig wraps a connector's parsed config as a node config.
func NewSourceConfig(connectorType string, cfg connector.Config) *SourceConfig {
	return &SourceConfig{connectorType: connectorType, inner: cfg}
}

// ConnectorType is the registered connector this node configures — the same
// string as the node's `type:`.
func (c *SourceConfig) ConnectorType() string { return c.connectorType }

// Connector returns the parsed connector config, which is what a factory
// needs to construct a running instance.
func (c *SourceConfig) Connector() connector.Config { return c.inner }

func (c *SourceConfig) Inputs() int  { return 0 }
func (c *SourceConfig) Outputs() int { return 1 }

func (c *SourceConfig) Validate(Refs) error {
	if c.inner == nil {
		return fmt.Errorf("source node: no connector config for type %q", c.connectorType)
	}
	return c.inner.Validate()
}

// MarshalYAML and MarshalJSON emit the connector's own fields, so a source
// node round-trips to exactly the flattened shape every other node type does
// — the wrapper is invisible on disk and on the wire.
func (c *SourceConfig) MarshalYAML() (any, error) { return c.inner, nil }

func (c *SourceConfig) MarshalJSON() ([]byte, error) { return json.Marshal(c.inner) }

// UnmarshalYAML re-creates the strict decode rather than delegating to the
// caller's decoder. A custom unmarshaler bypasses the outer decoder's
// KnownFields(true), so without this an unknown key in a source node's config
// would decode to nothing instead of failing the load — the silent-failure
// mode every other node type is protected from.
func (c *SourceConfig) UnmarshalYAML(value *yaml.Node) error {
	if c.inner == nil {
		return fmt.Errorf("source node: no connector config for type %q", c.connectorType)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(c.inner)
}

// UnmarshalJSON mirrors UnmarshalYAML: json.Unmarshal on a type with a custom
// unmarshaler drops the outer decoder's DisallowUnknownFields, so it is
// re-applied here.
func (c *SourceConfig) UnmarshalJSON(data []byte) error {
	if c.inner == nil {
		return fmt.Errorf("source node: no connector config for type %q", c.connectorType)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(c.inner)
}

// sourceNodeFactories is one registry entry per registered source connector.
// This is what makes adding a connector a change to cmd/desktop/internal/app/sources
// alone: the node type, its config decoding, and its port counts all follow
// from the descriptor, so nothing has to be added here.
func sourceNodeFactories() map[string]nodeFactory {
	out := make(map[string]nodeFactory, len(sources.All()))
	for connectorType, descriptor := range sources.All() {
		out[connectorType] = func() NodeConfig {
			return NewSourceConfig(descriptor.Type, descriptor.NewConfig())
		}
	}
	return out
}
