package connector

import (
	"fmt"
	"time"

	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"
)

// Duration is the duration a connector config declares — "30s", "1h". Like
// flow.Duration it rejects unquoted integer scalars, because a bare number in
// this schema is virtually always an author's mistaken assumption that it means
// seconds; unlike flow.Duration it lives here, because a connector config
// cannot import flow (flow imports the registry that imports the connectors).
type Duration time.Duration

// Duration returns the wrapped time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// String satisfies fmt.Stringer so a Duration prints like one in errors.
func (d Duration) String() string { return time.Duration(d).String() }

// MarshalText formats the duration as a Go duration string. Implementing
// encoding.TextMarshaler covers both wire formats with one method — yaml.v3
// and encoding/json each fall back to it — so a saved flow and the editor's
// JSON both carry "5m", never a bare nanosecond count.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalText parses a Go duration string, for the JSON decode the node
// editor round-trips through.
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
		return fmt.Errorf("duration: expected a string like \"30s\"")
	}
	switch value.Tag {
	case "!!int", "!!float":
		return fmt.Errorf("duration: %q must be a duration string like \"30s\", not a bare number", value.Value)
	}
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	return d.UnmarshalText([]byte(s))
}

// JSONSchema describes the wire form rather than the Go one. Without it the
// reflector reports the underlying int64 and every generated form, MCP tool
// schema and doc would ask for nanoseconds.
func (Duration) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:    "string",
		Pattern: `^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)([\d.]+(ns|us|µs|ms|s|m|h))*$`,
		Description: "A Go duration string such as \"30s\", \"5m\", or \"1h30m\". " +
			"A bare number is rejected rather than read as nanoseconds.",
	}
}
