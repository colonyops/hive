package relay

import (
	"errors"
	"time"
)

type Kind string

const (
	KindNotFound     Kind = "not_found"
	KindUnauthorized Kind = "unauthorized"
	KindForbidden    Kind = "forbidden"
	KindInvalid      Kind = "invalid"
	KindTooLarge     Kind = "too_large"
	KindRateLimited  Kind = "rate_limited"
	KindOverCap      Kind = "over_cap" // account bytes; 429 with no Retry-After
)

// Error is the one error type the services return. The sources-per-account
// cap is KindInvalid (422): creating one more is a bad request, not a
// transient condition.
type Error struct {
	Kind       Kind
	Message    string
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = string(e.Kind)
	}
	if e.Err != nil {
		return msg + ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf returns the kind of err when it is or wraps an *Error, else "".
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func NewError(kind Kind, msg string) *Error {
	return &Error{Kind: kind, Message: msg}
}
