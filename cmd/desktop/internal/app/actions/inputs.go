package actions

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The input control vocabulary. A value is always a string regardless of
// type; the type only chooses the control the invocation form renders.
const (
	InputTypeText      = "text"
	InputTypeSelect    = "select"
	InputTypeMultiline = "multiline"
)

var inputTypes = map[string]bool{
	InputTypeText:      true,
	InputTypeSelect:    true,
	InputTypeMultiline: true,
}

// inputNamePattern is Go's template field-selector rule, not the action-id
// slug rule: a declared input is read as `{{ .Inputs.<name> }}`, which the
// template parser accepts only for an identifier.
var inputNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// maxInputNameLen bounds a declared input name, mirroring maxSlugLen.
const maxInputNameLen = 64

// InputSpec declares one value collected from the user when an action is
// invoked, exposed to every template that action renders as
// `{{ .Inputs.<name> }}`.
//
// Inputs are an envelope field rather than per-type config: every action type
// renders over the same OutputData, so one declaration gives a new action type
// the invocation form with nothing further to wire (ADR action-declared-inputs).
type InputSpec struct {
	// Name is the template key: `{{ .Inputs.<name> }}`.
	Name string `json:"name" yaml:"name"`
	// Label is the form field's caption; the name is shown when it is empty.
	Label string `json:"label" yaml:"label,omitempty"`
	// Type selects the control: text, select, or multiline.
	Type string `json:"type" yaml:"type,omitempty"`
	// Required rejects an invocation whose value is blank.
	Required bool `json:"required" yaml:"required,omitempty"`
	// Default prefills the form and supplies the value when none is given.
	Default string `json:"default" yaml:"default,omitempty"`
	// Placeholder is the hint shown in an empty text or multiline control.
	Placeholder string `json:"placeholder" yaml:"placeholder,omitempty"`
	// Options is the closed value set of a select input.
	Options []string `json:"options" yaml:"options,omitempty"`
}

// displayLabel is the caption for this input, falling back to its name.
func (s InputSpec) displayLabel() string {
	if strings.TrimSpace(s.Label) != "" {
		return s.Label
	}
	return s.Name
}

// normalizeInputs applies the omitted-type default at the decode boundary, so
// nothing downstream — validation, the form, the YAML writer — has to treat a
// blank type as text a second time.
func normalizeInputs(specs []InputSpec) []InputSpec {
	for i := range specs {
		if specs[i].Type == "" {
			specs[i].Type = InputTypeText
		}
	}
	return specs
}

func validateInputs(specs []InputSpec) error {
	names := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if spec.Name == "" {
			return fmt.Errorf("inputs: name is required")
		}
		if len(spec.Name) > maxInputNameLen || !inputNamePattern.MatchString(spec.Name) {
			return fmt.Errorf("inputs: name %q must be a template identifier (letters, digits and underscores, not starting with a digit, max %d chars)", spec.Name, maxInputNameLen)
		}
		if names[spec.Name] {
			return fmt.Errorf("inputs: duplicate input name %q", spec.Name)
		}
		names[spec.Name] = true

		if !inputTypes[spec.Type] {
			return fmt.Errorf("inputs: input %q: unknown type %q", spec.Name, spec.Type)
		}
		if spec.Type != InputTypeSelect {
			if len(spec.Options) > 0 {
				return fmt.Errorf("inputs: input %q: options are only valid on a select input", spec.Name)
			}
			continue
		}
		if len(spec.Options) == 0 {
			return fmt.Errorf("inputs: input %q: a select input requires options", spec.Name)
		}
		seen := make(map[string]bool, len(spec.Options))
		for _, option := range spec.Options {
			if strings.TrimSpace(option) == "" {
				return fmt.Errorf("inputs: input %q: options must not be empty", spec.Name)
			}
			if seen[option] {
				return fmt.Errorf("inputs: input %q: duplicate option %q", spec.Name, option)
			}
			seen[option] = true
		}
		if spec.Default != "" && !slices.Contains(spec.Options, spec.Default) {
			return fmt.Errorf("inputs: input %q: default %q is not one of its options", spec.Name, spec.Default)
		}
	}
	return nil
}

// ResolveInputs validates one invocation's supplied values against this
// action's declared inputs and returns the map its templates render over:
// declared names only, blanks filled from their defaults.
//
// It is the single gate both the invocation preflight and the executing worker
// run, so what the detail pane accepts and what the templates see can never
// disagree. A supplied name the catalog does not declare is refused rather
// than ignored — it means the caller is working from a stale catalog.
func (a Action) ResolveInputs(supplied map[string]string) (map[string]string, error) {
	for name := range supplied {
		if !slices.ContainsFunc(a.Inputs, func(s InputSpec) bool { return s.Name == name }) {
			return nil, fmt.Errorf("action %q: unknown input %q", a.ID, name)
		}
	}
	if len(a.Inputs) == 0 {
		return nil, nil
	}

	resolved := make(map[string]string, len(a.Inputs))
	for _, spec := range a.Inputs {
		value := supplied[spec.Name]
		if strings.TrimSpace(value) == "" {
			value = spec.Default
		}
		if spec.Required && strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("action %q: input %q is required", a.ID, spec.displayLabel())
		}
		if spec.Type == InputTypeSelect && value != "" && !slices.Contains(spec.Options, value) {
			return nil, fmt.Errorf("action %q: input %q: %q is not one of its options", a.ID, spec.displayLabel(), value)
		}
		resolved[spec.Name] = value
	}
	return resolved, nil
}

// inputsHeadlessCapable reports whether every declared input can be satisfied
// without a user: a required input with no default has no value a flow worker
// could supply.
func (a Action) inputsHeadlessCapable() bool {
	for _, spec := range a.Inputs {
		if spec.Required && strings.TrimSpace(spec.Default) == "" {
			return false
		}
	}
	return true
}

func cloneInputs(specs []InputSpec) []InputSpec {
	if specs == nil {
		return nil
	}
	out := make([]InputSpec, len(specs))
	copy(out, specs)
	for i := range out {
		out[i].Options = append([]string(nil), specs[i].Options...)
	}
	return out
}
