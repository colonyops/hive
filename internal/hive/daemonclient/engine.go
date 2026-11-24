package daemonclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/hive/daemon/wire"
)

// EngineCall is one engine operation for the daemon to run.
type EngineCall struct {
	// Tool is the operation's MCP tool name, such as task_next.
	Tool string
	Args map[string]any
	// Read bounds the call as a read-only call is bounded, plus Wait for a
	// call that waits on purpose. A call that changes something has no bound,
	// as for sessions.
	Read bool
	Wait time.Duration
}

// CallEngine runs an engine operation on the daemon and answers its tool
// result, refused or not. It first refuses a daemon whose catalog this client
// does not call, with a *wire.CatalogError. An error means the call did not
// complete.
func (c *Client) CallEngine(ctx context.Context, call EngineCall) (*mcp.CallToolResult, error) {
	info, err := c.engineInfo(ctx)
	if err != nil {
		return nil, err
	}
	if err := wire.CheckCatalog(info.CatalogVersion, info.Version, c.clientVersion); err != nil {
		return nil, err
	}

	began := c.now()
	if call.Read && c.readTimeout > 0 {
		timeout := c.readTimeout
		if c.connectTimeout > 0 && c.openConns.Load() == 0 {
			timeout = c.connectTimeout
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout+call.Wait)
		defer cancel()
	}
	// The daemon's server is stateless, so there is no standalone stream to
	// open and no session to close. A negative MaxRetries leaves a retry to
	// the caller, which knows whether the call changes anything.
	transport := &mcp.StreamableClientTransport{
		Endpoint:             baseURL + wire.EnginePath,
		HTTPClient:           c.httpClient,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "hive", Version: c.clientVersion}, nil).Connect(ctx, transport, nil)
	if err != nil {
		return nil, c.explainEngine(ctx, err, call, began)
	}
	defer func() { _ = cs.Close() }()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: call.Tool, Arguments: call.Args})
	if err != nil {
		return nil, c.explainEngine(ctx, err, call, began)
	}
	return res, nil
}

// engineInfo is the Hello that the protocol gate checked, so an engine call
// on a checked connection costs no extra round trip.
func (c *Client) engineInfo(ctx context.Context) (Info, error) {
	if c.protocol != nil {
		return c.protocol.checkedInfo(ctx)
	}
	return c.Hello(ctx)
}

func (c *Client) explainEngine(ctx context.Context, err error, call EngineCall, began time.Time) error {
	target := c.name
	if target == "" {
		target = "the daemon"
	}
	var message string
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		message = fmt.Sprintf("%s did not answer in %s", target, c.now().Sub(began).Round(time.Second))
	case noDaemon(err) || transportFailure(err):
		message = "could not reach " + target
	case errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF):
		message = "lost the connection to " + target
	default:
		return fmt.Errorf("call %s on %s: %w", call.Tool, target, err)
	}
	if c.failureDetail != nil {
		if detail := c.failureDetail(); detail != "" {
			message += ": " + detail
		}
	}
	if !call.Read {
		message += "; the change may still finish on the server"
	}
	return wire.NewUnreachableError(c.name, message, err)
}
