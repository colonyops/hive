package flow

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Node is one node in a flow's graph: the common envelope fields (id, type,
// name, disabled) plus a per-type Config decoded via the registry.
type Node struct {
	ID       string
	Type     string
	Name     string
	Disabled bool
	Config   NodeConfig
}

// NodeConfig is the per-type union every registered node type implements.
// Inputs/Outputs report the node's port counts (used by wire validation);
// Validate checks the node's own fields, including any cross-file
// references, against the supplied Refs resolver.
type NodeConfig interface {
	Inputs() int
	Outputs() int
	Validate(refs Refs) error
}

// nodeFactory returns a fresh, zero-valued NodeConfig for a registered node
// type. Each call must return a distinct value (never a shared pointer) —
// the decoder mutates it in place.
type nodeFactory func() NodeConfig

// registry maps a node's `type:` discriminator to the factory for its
// per-type config. Registering a new node type means adding one entry here
// (and, if it's a terminal or source, nowhere else — Inputs/Outputs are
// carried by the config type itself).
var registry = map[string]nodeFactory{
	"github-source": func() NodeConfig { return &GithubSourceConfig{} },
	"rpc-source":    func() NodeConfig { return &RPCSourceConfig{} },
	"github-filter": func() NodeConfig { return &GithubFilterConfig{} },
	"function":      func() NodeConfig { return &FunctionConfig{} },
	"feed":          func() NodeConfig { return &FeedConfig{} },
	"action":        func() NodeConfig { return &ActionConfig{} },
}

// nodeHeader is the small set of fields common to every node, decoded first
// (laxly — unknown keys ignored) purely to read the `type:` discriminator
// and the envelope fields.
type nodeHeader struct {
	ID       string `yaml:"id"`
	Type     string `yaml:"type"`
	Name     string `yaml:"name"`
	Disabled bool   `yaml:"disabled"`
}

// reservedNodeKeys are the envelope keys every node mapping may carry.
// UnmarshalYAML strips these before the strict per-type decode so a node's
// own id/type/name/disabled fields never trip "unknown field" on the
// per-type config struct — none of the NodeConfig implementations need to
// declare (and ignore) them.
var reservedNodeKeys = map[string]bool{
	"id":       true,
	"type":     true,
	"name":     true,
	"disabled": true,
}

// UnmarshalYAML implements the two-pass strict decode: (1) decode a lax
// Header to read the `type:` discriminator, (2) look up the type in the
// registry (unknown type is a hard error), (3) strict-decode the node's
// remaining fields — with the reserved envelope keys stripped out first —
// into a fresh per-type config so an unknown per-type field is also a hard
// error.
func (n *Node) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("node: expected a mapping, got %s", nodeKindName(value.Kind))
	}

	var header nodeHeader
	if err := value.Decode(&header); err != nil {
		return fmt.Errorf("node: %w", err)
	}

	factory, ok := registry[header.Type]
	if !ok {
		if header.ID != "" {
			return fmt.Errorf("node %q: unknown type %q", header.ID, header.Type)
		}
		return fmt.Errorf("node: unknown type %q", header.Type)
	}
	cfg := factory()

	stripped := stripReservedKeys(value)
	data, err := yaml.Marshal(stripped)
	if err != nil {
		return fmt.Errorf("node %q (type %q): %w", header.ID, header.Type, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("node %q (type %q): %w", header.ID, header.Type, err)
	}

	n.ID = header.ID
	n.Type = header.Type
	n.Name = header.Name
	n.Disabled = header.Disabled
	n.Config = cfg
	return nil
}

// stripReservedKeys returns a shallow copy of a node's mapping node with the
// envelope keys (id/type/name/disabled) removed, leaving only per-type
// fields for the strict config decode.
func stripReservedKeys(value *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i]
		if reservedNodeKeys[key.Value] {
			continue
		}
		out.Content = append(out.Content, key, value.Content[i+1])
	}
	return out
}

func nodeKindName(kind yaml.Kind) string {
	switch kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "unknown"
	}
}
