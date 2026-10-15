package core

import "time"

type options struct {
	now func() time.Time
}

// Option configures Accounts and Tokens.
type Option func(*options)

// WithClock replaces time.Now, so tests can move time.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

func newOptions(opts []Option) options {
	o := options{now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
