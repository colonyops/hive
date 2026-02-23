package todo

import (
	"fmt"
	"strings"
)

// URI is a typed reference in scheme://value format.
// The scheme identifies the action type (e.g. "session", "review", "https")
// and the value identifies the specific resource.
type URI struct {
	Scheme string // e.g. "session", "review", "https"; always lowercase
	Value  string // opaque value, scheme-dependent
}

// ParseURI parses a raw string in scheme://value format into a URI.
// Schemes are normalized to lowercase. Bare strings without "://" are
// stored with an empty Scheme and Valid() returns false.
func ParseURI(raw string) URI {
	if raw == "" {
		return URI{}
	}
	scheme, value, ok := strings.Cut(raw, "://")
	if !ok {
		return URI{Value: raw}
	}
	return URI{
		Scheme: strings.ToLower(scheme),
		Value:  value,
	}
}

// String returns the URI in scheme://value format, or the bare value if no scheme.
func (u URI) String() string {
	if u.Scheme == "" {
		return u.Value
	}
	return u.Scheme + "://" + u.Value
}

// IsZero returns true if the URI has not been set.
func (u URI) IsZero() bool {
	return u.Scheme == "" && u.Value == ""
}

// Valid returns true when the URI is either empty or has a scheme.
// A non-empty value without a scheme is invalid (bare string).
func (u URI) Valid() bool {
	if u.IsZero() {
		return true
	}
	return u.Scheme != ""
}

// MarshalText implements encoding.TextMarshaler.
func (u URI) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (u *URI) UnmarshalText(data []byte) error {
	*u = ParseURI(string(data))
	return nil
}

// GoString implements fmt.GoStringer for debugging.
func (u URI) GoString() string {
	return fmt.Sprintf("todo.URI{Scheme: %q, Value: %q}", u.Scheme, u.Value)
}
