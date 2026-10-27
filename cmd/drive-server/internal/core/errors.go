package core

import "errors"

// Kind is the stable error category the HTTP adapter maps to a status.
type Kind string

const (
	KindInvalid         Kind = "invalid"
	KindForbidden       Kind = "forbidden"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindUnauthenticated Kind = "unauthenticated"
	KindTooLarge        Kind = "too_large"
	KindRateLimited     Kind = "rate_limited"
)

// Error is the error type services return for outcomes a caller can act on.
type Error struct {
	Kind    Kind
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

func NewError(kind Kind, msg string) *Error {
	return &Error{Kind: kind, Message: msg}
}

// ErrNotFound is what a store returns for a missing row. Services translate
// it into a message that names the thing when a caller sees it.
var ErrNotFound = NewError(KindNotFound, "not found")

func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
