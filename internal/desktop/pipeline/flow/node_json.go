package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// nodeEnvelopeJSON is Node's envelope fields for JSON, mirroring nodeHeader
// (node.go) for YAML — see UnmarshalYAML for why the envelope and the
// per-type config are decoded in two passes.
type nodeEnvelopeJSON struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// MarshalJSON flattens the envelope fields and the per-type config's own
// fields into one JSON object — the wire shape the desktop frontend's graph
// editor consumes over Wails (FlowsService.GetFlow/SaveFlow) — mirroring
// the flattened *.yaml on-disk shape produced by MarshalYAML.
func (n Node) MarshalJSON() ([]byte, error) {
	envelope, err := json.Marshal(nodeEnvelopeJSON{ID: n.ID, Type: n.Type, Name: n.Name, Disabled: n.Disabled})
	if err != nil {
		return nil, fmt.Errorf("node %q: %w", n.ID, err)
	}
	if n.Config == nil {
		return envelope, nil
	}

	cfgBytes, err := json.Marshal(n.Config)
	if err != nil {
		return nil, fmt.Errorf("node %q: encode config: %w", n.ID, err)
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(envelope, &merged); err != nil {
		return nil, fmt.Errorf("node %q: %w", n.ID, err)
	}
	var cfgFields map[string]json.RawMessage
	if err := json.Unmarshal(cfgBytes, &cfgFields); err != nil {
		return nil, fmt.Errorf("node %q: config type %T did not encode to an object", n.ID, n.Config)
	}
	for k, v := range cfgFields {
		merged[k] = v
	}
	return json.Marshal(merged)
}

// UnmarshalJSON implements the same two-pass strict decode as UnmarshalYAML:
// (1) decode the envelope to read the `type` discriminator, (2) look up the
// type in the registry (unknown type is a hard error), (3) strict-decode
// the remaining fields — with the reserved envelope keys stripped out first
// — into a fresh per-type config, so an unrecognized per-type field is also
// a hard error (matching UnmarshalYAML's KnownFields(true) behavior).
func (n *Node) UnmarshalJSON(data []byte) error {
	var envelope nodeEnvelopeJSON
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("node: %w", err)
	}

	factory, ok := registry[envelope.Type]
	if !ok {
		if envelope.ID != "" {
			return fmt.Errorf("node %q: unknown type %q", envelope.ID, envelope.Type)
		}
		return fmt.Errorf("node: unknown type %q", envelope.Type)
	}
	cfg := factory()

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("node %q: %w", envelope.ID, err)
	}
	for key := range reservedNodeKeys {
		delete(raw, key)
	}
	stripped, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("node %q: %w", envelope.ID, err)
	}
	dec := json.NewDecoder(bytes.NewReader(stripped))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("node %q (type %q): %w", envelope.ID, envelope.Type, err)
	}

	n.ID = envelope.ID
	n.Type = envelope.Type
	n.Name = envelope.Name
	n.Disabled = envelope.Disabled
	n.Config = cfg
	return nil
}
