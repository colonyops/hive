// Package actions implements the desktop pipeline's actions.yml schema: the
// output/action layer a flow's terminal `action` node targets (see
// internal/desktop/pipeline/flow's ActionConfig) and the source for the
// desktop detail pane's action buttons.
//
// It mirrors the flow package's registry + two-pass strict decode: a
// discriminated union over `type`, probed via a lax header, dispatched to a
// per-type config via a registry, then strictly re-decoded (KnownFields)
// with the reserved envelope keys stripped so unknown per-type fields still
// fail. Like flow, this package is self-contained — it does not import flow
// (nor is it imported by flow); a caller (main.go) wires an ActionStore into
// flow.Refs once both are loaded.
package actions

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// View is the complete action contract exposed to the desktop frontend. It
// deliberately excludes executable configuration: the frontend can present
// and identify an action, but execution always resolves its current
// definition from ActionStore.
type View struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	AutoApply bool   `json:"autoApply"`
}

// Action is one parsed and validated actions.yml entry: the common envelope
// fields plus a per-type Config decoded via the registry.
type Action struct {
	// ID is the action's id, referenced by a flow's `action:` node field and
	// by the detail-pane action picker.
	ID string
	// Label is the human-readable name shown wherever the action is offered
	// (a flow node, a detail-pane button).
	Label string
	// Type is the action kind discriminator: "launch-session", "shell", or
	// "publish-event".
	Type string
	// AppliesTo restricts which feed item kinds this action is offered for in
	// the detail pane; empty means "any kind". It plays no role in flow
	// `action` nodes, which target one specific action id explicitly
	// regardless of kind.
	AppliesTo []string
	// AutoApply, when true, lets the output worker execute this action's
	// queued output_command rows automatically. When false (the default),
	// the worker moves a queued command to awaiting_confirmation until a
	// manual confirmation UI (the detail pane) triggers it.
	AutoApply bool
	// Config is the per-type configuration: *LaunchSessionConfig,
	// *ShellConfig, or *PublishEventConfig.
	Config ActionConfig
}

// View returns the safe presentation contract for this action.
func (a Action) View() View {
	return View{ID: a.ID, Label: a.Label, Type: a.Type, AutoApply: a.AutoApply}
}

// ActionConfig is the per-type union every registered action type
// implements. Validate checks the type's own required/well-formed fields;
// it does not have access to other actions (dup-id checking is a
// whole-file concern, done by validateActions).
type ActionConfig interface {
	Validate() error
}

// actionFactory returns a fresh, zero-valued ActionConfig for a registered
// action type. Each call must return a distinct value (never a shared
// pointer) — the decoder mutates it in place.
type actionFactory func() ActionConfig

// registry maps an action's `type:` discriminator to the factory for its
// per-type config.
var registry = map[string]actionFactory{
	"launch-session": func() ActionConfig { return &LaunchSessionConfig{} },
	"shell":          func() ActionConfig { return &ShellConfig{} },
	"publish-event":  func() ActionConfig { return &PublishEventConfig{} },
}

// actionHeader is the small set of fields common to every action, decoded
// first (laxly — unknown keys ignored) purely to read the `type:`
// discriminator and the envelope fields.
type actionHeader struct {
	ID        string   `yaml:"id"`
	Label     string   `yaml:"label"`
	Type      string   `yaml:"type"`
	AppliesTo []string `yaml:"applies_to"`
	AutoApply bool     `yaml:"auto_apply"`
}

// reservedActionKeys are the envelope keys every action mapping may carry.
// UnmarshalYAML strips these before the strict per-type decode so an
// action's own envelope fields never trip "unknown field" on the per-type
// config struct.
var reservedActionKeys = map[string]bool{
	"id":         true,
	"label":      true,
	"type":       true,
	"applies_to": true,
	"auto_apply": true,
}

// UnmarshalYAML implements the two-pass strict decode: (1) decode a lax
// Header to read the `type:` discriminator, (2) look up the type in the
// registry (unknown type is a hard error), (3) strict-decode the action's
// remaining fields — with the reserved envelope keys stripped out first —
// into a fresh per-type config, so an unknown per-type field is also a hard
// error.
func (a *Action) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("action: expected a mapping, got %s", nodeKindName(value.Kind))
	}

	var header actionHeader
	if err := value.Decode(&header); err != nil {
		return fmt.Errorf("action: %w", err)
	}

	factory, ok := registry[header.Type]
	if !ok {
		if header.ID != "" {
			return fmt.Errorf("action %q: unknown type %q", header.ID, header.Type)
		}
		return fmt.Errorf("action: unknown type %q", header.Type)
	}
	cfg := factory()

	stripped := stripReservedKeys(value)
	data, err := yaml.Marshal(stripped)
	if err != nil {
		return fmt.Errorf("action %q (type %q): %w", header.ID, header.Type, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("action %q (type %q): %w", header.ID, header.Type, err)
	}

	a.ID = header.ID
	a.Label = header.Label
	a.Type = header.Type
	a.AppliesTo = header.AppliesTo
	a.AutoApply = header.AutoApply
	a.Config = cfg
	return nil
}

// stripReservedKeys returns a shallow copy of an action's mapping node with
// the envelope keys (id/label/type/applies_to/auto_apply) removed, leaving
// only per-type fields for the strict config decode.
func stripReservedKeys(value *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i]
		if reservedActionKeys[key.Value] {
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
