package core

import (
	"strings"
	"time"
)

type options struct {
	now     func() time.Time
	baseURL string
}

type Option func(*options)

func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// WithBaseURL sets the public origin that share URLs are built on.
func WithBaseURL(u string) Option {
	return func(o *options) { o.baseURL = strings.TrimRight(u, "/") }
}

func newOptions(opts []Option) options {
	o := options{now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
