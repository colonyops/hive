package daemonclient

import (
	"context"
	"errors"
	"net"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
)

const (
	firstBackoff = time.Second
	maxBackoff   = 30 * time.Second
)

// starter starts the daemon when a dial finds none, at most once at a time.
// After a failed start it returns that failure without trying again until
// the backoff, which doubles from 1s up to 30s, has passed.
type starter struct {
	start func(ctx context.Context) error
	now   func() time.Time

	mu        sync.Mutex
	backoff   time.Duration
	retryAt   time.Time
	lastError error
}

// ensure starts the daemon unless up reports one already answering, as it
// does for every caller that waited on the lock behind a successful start.
func (s *starter) ensure(ctx context.Context, up func() bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if up() {
		return nil
	}
	if s.lastError != nil && s.now().Before(s.retryAt) {
		return s.lastError
	}
	if err := s.start(ctx); err != nil {
		if s.backoff == 0 {
			s.backoff = firstBackoff
		} else {
			s.backoff = min(2*s.backoff, maxBackoff)
		}
		s.retryAt = s.now().Add(s.backoff)
		s.lastError = err
		return err
	}
	s.backoff = 0
	s.lastError = nil
	return nil
}

// noDaemon reports a dial that found no daemon: no socket file, or a socket
// a dead daemon left behind.
func noDaemon(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED)
}

// Dial opens a raw connection to the daemon, starting one when AutoStart is
// set and none answers. `hive daemon dial-stdio` bridges it to ssh.
func (c *Client) Dial(ctx context.Context) (net.Conn, error) { return c.dial(ctx) }

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if c.protocol != nil {
		c.protocol.reset()
	}
	conn, err := c.dialer(ctx)
	if err == nil || c.starter == nil || !noDaemon(err) {
		return conn, err
	}
	up := func() bool {
		conn, err := c.dialer(ctx)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
	if startErr := c.starter.ensure(ctx, up); startErr != nil {
		return nil, startErr
	}
	return c.dialer(ctx)
}

// transportFailure reports an error that may have come before the daemon
// saw the request, such as a connection to a daemon that died. A read can
// run again after one; a mutation cannot, since it may have applied.
// Connect codes a write to a closed socket as Canceled, so the errno counts
// too.
func transportFailure(err error) bool {
	return connect.CodeOf(err) == connect.CodeUnavailable ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

// retryRead runs a read, and once more after a transport failure.
func retryRead[T any](call func() (T, error)) (T, error) {
	result, err := call()
	if err != nil && transportFailure(err) {
		return call()
	}
	return result, err
}
