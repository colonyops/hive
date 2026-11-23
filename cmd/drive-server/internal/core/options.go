package core

import (
	"strings"
	"time"
)

type options struct {
	now     func() time.Time
	baseURL string
	notify  Notifier
	caps    CommentCaps
}

type Option func(*options)

func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// WithBaseURL sets the public origin that share URLs are built on.
func WithBaseURL(u string) Option {
	return func(o *options) { o.baseURL = strings.TrimRight(u, "/") }
}

// WithNotifier sets who hears about document and thread changes.
func WithNotifier(n Notifier) Option {
	return func(o *options) { o.notify = n }
}

func WithCommentCaps(c CommentCaps) Option {
	return func(o *options) { o.caps = c }
}

func newOptions(opts []Option) options {
	o := options{now: time.Now, notify: noopNotifier{}, caps: DefaultCommentCaps}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
