package daemonclient

import (
	"context"
	"strings"
	"sync"

	"connectrpc.com/connect"

	"github.com/colonyops/hive/internal/hive/daemon/wire"
)

// protocolGate runs Hello before the first other call on each new connection
// and refuses every call while the daemon's protocol is incompatible. A new
// connection checks again, since the daemon may have been upgraded.
type protocolGate struct {
	client        *Client
	clientVersion string

	mu      sync.Mutex
	checked bool
	err     error
}

func (g *protocolGate) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checked = false
	g.err = nil
}

func (g *protocolGate) check(ctx context.Context) error {
	g.mu.Lock()
	checked, err := g.checked, g.err
	g.mu.Unlock()
	if checked {
		return err
	}
	// Not under mu: Hello may dial, and a dial calls reset. One attempt: the read
	// behind this check retries by itself, and two retries multiply the wait.
	info, err := g.client.helloOnce(ctx)
	if err != nil {
		// Not recorded: a transport failure says nothing about the protocol.
		return err
	}
	err = wire.CheckProtocol(info.ProtocolVersion, info.Version, g.clientVersion)
	g.mu.Lock()
	g.checked, g.err = true, err
	g.mu.Unlock()
	return err
}

func isHello(procedure string) bool {
	return strings.HasSuffix(procedure, "/Hello")
}

func (g *protocolGate) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !isHello(req.Spec().Procedure) {
			if err := g.check(ctx); err != nil {
				return nil, err
			}
		}
		return next(ctx, req)
	}
}

func (g *protocolGate) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		if err := g.check(ctx); err != nil {
			return &failedStream{StreamingClientConn: next(ctx, spec), err: err}
		}
		return next(ctx, spec)
	}
}

func (g *protocolGate) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// failedStream reports err from every send and receive.
type failedStream struct {
	connect.StreamingClientConn
	err error
}

func (s *failedStream) Send(any) error    { return s.err }
func (s *failedStream) Receive(any) error { return s.err }
