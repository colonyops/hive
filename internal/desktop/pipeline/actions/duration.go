package actions

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration decodes a Go duration string ("5s", "250ms") from YAML. Unlike a
// bare time.Duration, it rejects unquoted integer scalars — "timeout: 5" is
// a hard error, not "5 nanoseconds" — for the same reason
// flow.Duration does: a bare number in this schema is virtually always an
// author's mistaken assumption that it means seconds.
//
// This is a deliberate copy of flow.Duration rather than a cross-package
// reuse: actions.yml is its own schema package, decoded independently of
// flows/*.yaml, and keeping the two schema packages free of dependencies on
// each other (actions never imports flow, flow never imports actions) keeps
// each one loadable/testable in isolation — the same posture flow's own
// package doc describes for its relationship to profiles/actions.
type Duration time.Duration

// Duration returns the wrapped time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// String satisfies fmt.Stringer so Duration prints like a duration in error
// messages.
func (d Duration) String() string {
	return time.Duration(d).String()
}

// UnmarshalYAML rejects bare integer/float scalars and otherwise parses the
// scalar as a Go duration string.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration: expected a string like \"5s\"")
	}
	switch value.Tag {
	case "!!int", "!!float":
		return fmt.Errorf("duration: %q must be a duration string like \"5s\", not a bare number", value.Value)
	}
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	*d = Duration(parsed)
	return nil
}
