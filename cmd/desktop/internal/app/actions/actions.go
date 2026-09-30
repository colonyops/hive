// Package actions implements the desktop pipeline's actions.yml schema: the
// output/action layer a flow's terminal `action` node targets (see
// internal/app/flow's ActionConfig) and the source for the
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
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The surfaces an action can be offered on. An action names them in
// `targets:`; declaring none means TargetItem, which is what every action
// written before the terminal surfaces existed means.
//
// `applies_to` and `show_in_detail` refine the item surface only: a terminal
// target has no item kind to match against, and its presence in `targets` is
// already the decision to offer it.
const (
	// TargetItem is a feed item — the detail pane and the item action menu.
	TargetItem = "item"
	// TargetSession is a terminal session's row menu.
	TargetSession = "session"
	// TargetWindow is a terminal window's row menu, inside its session.
	TargetWindow = "window"
)

var targetNames = map[string]bool{TargetItem: true, TargetSession: true, TargetWindow: true}

// View is the complete action contract exposed to the desktop frontend. It
// deliberately excludes executable configuration: the frontend can present
// and identify an action, but execution always resolves its current
// definition from ActionStore.
type View struct {
	ID                   string      `json:"id"`
	Label                string      `json:"label"`
	Type                 string      `json:"type"`
	ShowInDetail         bool        `json:"showInDetail"`
	RequiresSessionInput bool        `json:"requiresSessionInput"`
	Inputs               []InputSpec `json:"inputs,omitempty"`
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
	// Type is the action kind discriminator: "launch-session", "shell",
	// "publish-message", or "clipboard".
	Type string
	// Targets are the surfaces this action is offered on: TargetItem,
	// TargetSession, TargetWindow. Empty means TargetItem alone.
	Targets []string
	// AppliesTo restricts which feed item kinds this action is offered for in
	// the detail pane; empty means "any kind". It plays no role in flow
	// `action` nodes, which target one specific action id explicitly
	// regardless of kind, nor in the terminal targets, which have no item.
	AppliesTo []string
	// ShowInDetail controls whether this action is offered in the detail pane.
	// Flow action nodes remain eligible regardless of this presentation flag.
	ShowInDetail bool
	// Inputs declares the values collected from the user at invocation time
	// and rendered into this action's templates as `.Inputs.<name>`.
	Inputs []InputSpec
	// Config is the per-type configuration: *LaunchSessionConfig,
	// *ShellConfig, *PublishMessageConfig, or *ClipboardConfig.
	Config ActionConfig
}

// View returns the safe presentation contract for this action.
func (a Action) View() View {
	return View{
		ID:                   a.ID,
		Label:                a.Label,
		Type:                 a.Type,
		ShowInDetail:         a.ShowInDetail,
		RequiresSessionInput: a.RequiresSessionInput(),
		Inputs:               cloneInputs(a.Inputs),
	}
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
	"launch-session":  func() ActionConfig { return &LaunchSessionConfig{} },
	"shell":           func() ActionConfig { return &ShellConfig{} },
	"publish-message": func() ActionConfig { return &PublishMessageConfig{} },
	"clipboard":       func() ActionConfig { return &ClipboardConfig{} },
}

// actionHeader is the small set of fields common to every action, decoded
// first (laxly — unknown keys ignored) purely to read the `type:`
// discriminator and the envelope fields.
type actionHeader struct {
	ID           string      `yaml:"id"`
	Label        string      `yaml:"label"`
	Type         string      `yaml:"type"`
	Targets      []string    `yaml:"targets"`
	AppliesTo    []string    `yaml:"applies_to"`
	ShowInDetail bool        `yaml:"show_in_detail"`
	Inputs       []InputSpec `yaml:"inputs"`
}

// reservedActionKeys are the envelope keys every action mapping may carry.
// UnmarshalYAML strips these before the strict per-type decode so an
// action's own envelope fields never trip "unknown field" on the per-type
// config struct.
var reservedActionKeys = map[string]bool{
	"id":             true,
	"label":          true,
	"type":           true,
	"targets":        true,
	"applies_to":     true,
	"show_in_detail": true,
	"inputs":         true,
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
	a.Targets = normalizeTargets(header.Targets)
	a.AppliesTo = header.AppliesTo
	a.ShowInDetail = header.ShowInDetail
	a.Inputs = normalizeInputs(header.Inputs)
	a.Config = cfg
	return nil
}

// stripReservedKeys returns a shallow copy of an action's mapping node with
// the envelope keys (reservedActionKeys) removed, leaving only per-type
// fields for the strict config decode.
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

// normalizeTargets lowercases and trims the declared surfaces at the decode
// boundary, so validation and HasTarget compare against the vocabulary
// exactly once. A declaration of TargetItem alone collapses to the empty
// default, which is the same thing and keeps it out of the YAML writer.
func normalizeTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, strings.ToLower(strings.TrimSpace(target)))
	}
	if len(out) == 1 && out[0] == TargetItem {
		return nil
	}
	return out
}

// HasTarget reports whether this action is offered on target.
func (a Action) HasTarget(target string) bool {
	if len(a.Targets) == 0 {
		return target == TargetItem
	}
	return slices.Contains(a.Targets, target)
}

// TargetsTerminal reports whether this action is offered on either terminal
// surface.
func (a Action) TargetsTerminal() bool {
	return a.HasTarget(TargetSession) || a.HasTarget(TargetWindow)
}

// TerminalCapable reports whether this action's type can run against a
// terminal target at all. It sits beside HeadlessCapable because it answers
// the same shape of question — which surfaces a type is executable on — and
// validateActions refuses a declaration that contradicts it, so the refusal
// lands when the catalog is authored rather than when the menu entry is
// clicked.
func (a Action) TerminalCapable() bool {
	switch a.Config.(type) {
	case *LaunchSessionConfig:
		// Launching creates a *new* session. Its interactive variant needs the
		// New Session form the terminal surface does not have, and its headless
		// variant's repo_template renders over a feed item's payload, which a
		// terminal target carries none of.
		return false
	default:
		return true
	}
}

// HeadlessCapable reports whether a flow worker can execute this action without interactive input.
func (a Action) HeadlessCapable() bool {
	if !a.inputsHeadlessCapable() {
		return false
	}
	switch c := a.Config.(type) {
	case *LaunchSessionConfig:
		return strings.TrimSpace(c.RepoTemplate) != "" || strings.TrimSpace(c.Workspace) != ""
	case *ClipboardConfig:
		// A clipboard action has no clipboard target from a headless flow, so
		// it is a detail-pane affordance only and never a flow terminal.
		return false
	default:
		return true
	}
}

// Clipboard actions qualify when all inputs resolve unasked.
func (a Action) RunsWithoutInput() bool {
	if _, clipboard := a.Config.(*ClipboardConfig); clipboard {
		return a.inputsHeadlessCapable()
	}
	return a.HeadlessCapable()
}

func (a Action) RequiresSessionInput() bool {
	c, ok := a.Config.(*LaunchSessionConfig)
	return ok && strings.TrimSpace(c.RepoTemplate) == "" && strings.TrimSpace(c.Workspace) == ""
}
