// Package app is the headless core's facade: the App value driving adapters
// hold, the error vocabulary they translate, and the per-domain services that
// own orchestration. The domain packages it composes live in
// internal/app/<domain>/.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Kind is the transport-neutral classification of a core failure. Each adapter
// maps Kind to its own vocabulary exactly once, and nothing anywhere may match
// on error text.
type Kind string

const (
	// KindInternal is the zero classification: something broke and the caller
	// can do nothing about it.
	KindInternal Kind = "internal"
	// KindInvalid means the caller supplied bad input.
	KindInvalid Kind = "invalid"
	// KindNotFound means the named thing does not exist.
	KindNotFound Kind = "not_found"
	// KindConflict means the caller's view is stale: an inbox item revision or
	// an action catalog order that moved underneath them. Re-read and retry.
	KindConflict Kind = "conflict"
	// KindUnauthenticated means the operation needs GitHub credentials that
	// are absent, rejected, or expired. The user's remedy is to sign in.
	KindUnauthenticated Kind = "unauthenticated"
	// KindUnavailable means the operation is temporarily or structurally
	// impossible through no fault of the caller: a subsystem not wired in this
	// mock mode, or an upstream cooldown. Retrying later may work; signing in
	// will not. Rate limiting is deliberately here rather than under
	// KindUnauthenticated — the token is fine, the quota is not.
	KindUnavailable Kind = "unavailable"
)

// AllKinds is the closed set, in declaration order. It exists so a boundary
// that has to mirror the vocabulary — the frontend's AppErrorKind union — can
// be checked against it rather than kept in sync by hand.
func AllKinds() []Kind {
	return []Kind{KindInternal, KindInvalid, KindNotFound, KindConflict, KindUnauthenticated, KindUnavailable}
}

// Error is the only error type an app service returns across the adapter
// boundary. Msg is safe to show a user; Err is the cause, kept for logs and
// for errors.Is/As.
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	return e.Msg + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// MarshalJSON emits {"kind":…,"message":…} so a driving adapter can put the
// Kind on the wire without re-deriving it. The cause is deliberately absent:
// it is for the log, not for the frontend.
func (e *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Kind    Kind   `json:"kind"`
		Message string `json:"message"`
	}{Kind: e.Kind, Message: e.Msg})
}

// Errorf classifies a new failure with no cause.
func Errorf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// Wrap classifies an existing failure. It returns nil for a nil err so a
// caller can wrap unconditionally — which is the whole point of it, and the
// reason it returns error rather than *Error: a nil *Error assigned to an
// error return is a non-nil interface holding a nil pointer, so every
// `return x, Wrap(err, ...)` would report success as a failure.
func Wrap(err error, kind Kind, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...), Err: err}
}

// KindOf returns the Kind of the first *Error in err's chain, or KindInternal.
// An unclassified error is internal by definition: nobody decided otherwise.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind
	}
	return KindInternal
}
