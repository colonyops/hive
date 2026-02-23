package todo

import (
	"fmt"
	"strings"
)

// Ref is a typed URI reference for a todo item.
// Format: "scheme://value" where scheme identifies the action type.
type Ref struct {
	Scheme string // e.g. "session", "review", "https"; always lowercase
	Value  string // opaque value, scheme-dependent
}

// ParseRef parses a URI string into scheme and value.
// Schemes are normalized to lowercase.
func ParseRef(raw string) Ref {
	if raw == "" {
		return Ref{}
	}
	scheme, value, ok := strings.Cut(raw, "://")
	if !ok {
		return Ref{Value: raw}
	}
	return Ref{
		Scheme: strings.ToLower(scheme),
		Value:  value,
	}
}

// String returns the wire format "scheme://value".
func (r Ref) String() string {
	if r.Scheme == "" {
		return r.Value
	}
	return r.Scheme + "://" + r.Value
}

// IsEmpty returns true when no ref is set.
func (r Ref) IsEmpty() bool {
	return r.Scheme == "" && r.Value == ""
}

// Valid returns true when the ref is either empty or has a scheme.
// A non-empty value without a scheme is invalid (bare string).
func (r Ref) Valid() bool {
	if r.IsEmpty() {
		return true
	}
	return r.Scheme != ""
}

// MarshalText implements encoding.TextMarshaler.
func (r Ref) MarshalText() ([]byte, error) {
	return []byte(r.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (r *Ref) UnmarshalText(data []byte) error {
	*r = ParseRef(string(data))
	return nil
}

// GoString implements fmt.GoStringer for debugging.
func (r Ref) GoString() string {
	return fmt.Sprintf("todo.Ref{Scheme: %q, Value: %q}", r.Scheme, r.Value)
}
