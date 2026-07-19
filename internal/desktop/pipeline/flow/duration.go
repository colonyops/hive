package flow

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration decodes a Go duration string ("5s", "250ms") from YAML. Unlike a
// bare time.Duration, it rejects unquoted integer scalars — "timeout: 5" is
// a hard error, not "5 nanoseconds" — because a bare number in this schema
// is virtually always an author's mistaken assumption that it means seconds.
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

// MarshalText formats the duration as a Go duration string ("5s") — the
// same format UnmarshalYAML and UnmarshalText require on read. Implementing
// encoding.TextMarshaler (rather than yaml.Marshaler directly) covers both
// wire formats with one method: yaml.v3 falls back to TextMarshaler when a
// type isn't a yaml.Marshaler, and encoding/json does the same for
// encoding.TextMarshaler/TextUnmarshaler — so SaveFlow's YAML output and the
// Wails JSON bridge both get "5s", never a bare nanosecond count.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalText parses a Go duration string, for JSON decode (the frontend
// graph editor's wire format via Wails). YAML decode goes through
// UnmarshalYAML instead, which additionally rejects bare integers — a
// distinction that matters for hand-authored YAML but not for a value the
// frontend already round-tripped through this same Duration type.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	*d = Duration(parsed)
	return nil
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
