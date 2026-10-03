package todo

import (
	"fmt"
	"strings"
)

// Ref is a typed URI reference for a todo item.
// Format: "scheme://value" where scheme identifies the action type.
// Fields are unexported; use Scheme() and Value() getters.
type Ref struct {
	scheme string // e.g. "session", "review", "https"; always lowercase
	value  string // opaque value, scheme-dependent
}

// Scheme returns the URI scheme (e.g. "session", "review", "https").
func (r Ref) Scheme() string { return r.scheme }

// Value returns the scheme-dependent value portion of the URI.
func (r Ref) Value() string { return r.value }

// ParseRef parses a URI string into scheme and value.
// Schemes are normalized to lowercase.
// Returns an error if the string is non-empty but has no "://" separator.
func ParseRef(raw string) (Ref, error) {
	if raw == "" {
		return Ref{}, nil
	}
	scheme, value, ok := strings.Cut(raw, "://")
	if !ok {
		return Ref{}, fmt.Errorf("invalid URI %q: must use scheme://value format", raw)
	}
	return Ref{
		scheme: strings.ToLower(scheme),
		value:  value,
	}, nil
}

// String returns the wire format "scheme://value".
func (r Ref) String() string {
	if r.scheme == "" {
		return r.value
	}
	return r.scheme + "://" + r.value
}

// IsEmpty returns true when no ref is set.
func (r Ref) IsEmpty() bool {
	return r.scheme == "" && r.value == ""
}

// Valid returns true when the ref is either empty or has a scheme.
// A non-empty value without a scheme is invalid (bare string).
func (r Ref) Valid() bool {
	if r.IsEmpty() {
		return true
	}
	return r.scheme != ""
}

// MarshalText implements encoding.TextMarshaler.
func (r Ref) MarshalText() ([]byte, error) {
	return []byte(r.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (r *Ref) UnmarshalText(data []byte) error {
	parsed, err := ParseRef(string(data))
	if err != nil {
		// For backward compatibility with stored bare strings, fall back gracefully.
		// The Ref will be invalid (Valid() returns false) but won't lose data.
		r.scheme = ""
		r.value = string(data)
		return nil
	}
	*r = parsed
	return nil
}

// GoString implements fmt.GoStringer for debugging.
func (r Ref) GoString() string {
	return fmt.Sprintf("todo.Ref{scheme: %q, value: %q}", r.scheme, r.value)
}

// MustParseRef is like ParseRef but panics on error.
// Use only in tests and initialization code.
func MustParseRef(raw string) Ref {
	ref, err := ParseRef(raw)
	if err != nil {
		panic(err)
	}
	return ref
}
