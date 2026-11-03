package ops

import (
	"fmt"
	"strings"
)

// Kind is the error vocabulary every surface maps once. A tool error crosses
// MCP as text that starts with "<kind>: ".
type Kind string

const (
	KindInvalid         Kind = "invalid"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindUnavailable     Kind = "unavailable"
	KindUnauthenticated Kind = "unauthenticated"
	KindInternal        Kind = "internal"
)

var exitCodes = map[Kind]int{
	KindInvalid:         2,
	KindNotFound:        3,
	KindConflict:        4,
	KindUnavailable:     5,
	KindUnauthenticated: 6,
	KindInternal:        1,
}

// ExitCode is the CLI exit status for a refused call of kind k. An unknown
// kind exits 1.
func ExitCode(k Kind) int {
	if code, ok := exitCodes[k]; ok {
		return code
	}
	return 1
}

// ParseKind reads the leading "<kind>:" of a tool error's text.
func ParseKind(msg string) (Kind, bool) {
	head, _, found := strings.Cut(msg, ":")
	if !found {
		return "", false
	}
	k := Kind(head)
	_, ok := exitCodes[k]
	return k, ok
}

// Error is an error with a Kind.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return string(e.Kind) + ": " + e.Msg }

func Errorf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}
