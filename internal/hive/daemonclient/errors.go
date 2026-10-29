package daemonclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/colonyops/hive/internal/hive/daemon/wire"
)

func (c *Client) explainFailures() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			began := c.now()
			resp, err := next(ctx, req)
			if err != nil {
				return resp, c.explain(err, req.Spec().Procedure, c.now().Sub(began))
			}
			return resp, nil
		}
	}
}

// streamFailure is explainFailures for a streaming call, which a unary
// interceptor does not see.
func (c *Client) streamFailure(err error, procedure string, began time.Time) error {
	if err == nil {
		return nil
	}
	return wire.FromConnectError(c.explain(err, procedure, c.now().Sub(began)))
}

func (c *Client) explain(err error, procedure string, elapsed time.Duration) error {
	var unreachable *wire.UnreachableError
	if errors.As(err, &unreachable) {
		return err
	}
	// The client's deadline travels with the call, so the daemon's own timeout
	// can answer first. That is still a daemon that did not answer in time.
	timedOut := connect.CodeOf(err) == connect.CodeDeadlineExceeded
	if connect.IsWireError(err) && !timedOut {
		return err
	}
	target := c.name
	if target == "" {
		target = "the daemon"
	}

	var message string
	switch {
	case timedOut:
		message = fmt.Sprintf("%s did not answer in %s", target, elapsed.Round(time.Second))
	case transportFailure(err):
		message = "could not reach " + target
	case errors.Is(err, io.ErrUnexpectedEOF):
		message = "lost the connection to " + target
	default:
		return err
	}

	if detail := c.detail(err); detail != "" {
		message += ": " + detail
	}
	if !readProcedures[procedure] {
		message += "; the change may still finish on the server"
	}
	return wire.NewUnreachableError(c.name, message, err)
}

// detail prefers the transport's own reason. Without one, only a failed
// connection has a reason worth showing: a timeout says all it can.
func (c *Client) detail(err error) string {
	if c.failureDetail != nil {
		if detail := c.failureDetail(); detail != "" {
			return detail
		}
	}
	var connectErr *connect.Error
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded && errors.As(err, &connectErr) {
		return connectErr.Message()
	}
	return ""
}

// countedConn reports its close once, however many times the transport closes
// it.
type countedConn struct {
	net.Conn
	once   sync.Once
	closed func()
}

func (c *countedConn) Close() error {
	c.once.Do(c.closed)
	return c.Conn.Close()
}
