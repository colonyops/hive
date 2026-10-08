// Package duration provides a time.Duration that crosses JSON as a Go
// duration string ("5s") rather than a nanosecond count.
//
// Plain time.Duration is enough for YAML-only config: yaml.v3 already reads
// and writes duration strings and rejects bare numbers. Use Duration when the
// value also travels as JSON or is described by a reflected JSON schema.
package duration

import (
	"fmt"
	"time"

	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"
)

type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d Duration) String() string { return time.Duration(d).String() }

// MarshalText also serves yaml.v3, which falls back to encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	*d = Duration(parsed)
	return nil
}

// UnmarshalYAML rejects bare numbers, which UnmarshalText alone would accept
// for "0", and names the mistake: an author who writes "timeout: 5" almost
// always means seconds.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration: expected a string like \"5s\"")
	}
	switch value.Tag {
	case "!!int", "!!float":
		return fmt.Errorf("duration: %q must be a duration string like \"5s\", not a bare number", value.Value)
	}
	return d.UnmarshalText([]byte(value.Value))
}

// JSONSchema describes the wire form; without it the reflector reports the
// underlying int64.
func (Duration) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:    "string",
		Pattern: `^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)([\d.]+(ns|us|µs|ms|s|m|h))*$`,
	}
}
