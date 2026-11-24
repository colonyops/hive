package commands

import (
	"context"
	"errors"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
)

// engineOpener builds the engine handlers and answers the function that
// closes what it opened.
type engineOpener func(ctx context.Context) (*engineops.Handlers, func() error, error)

// runsInProcess reports whether hive mcp serve runs an operation on its own
// engine. Session control stays with the desktop: its server is guarded by a
// token, which is the grant a workspace holds for it.
func runsInProcess(info ops.Info) bool {
	return info.Owner == ops.OwnerEngine && info.Toolset != sessions.ControlToolset
}

// localEngine serves the engine operations of hive mcp serve. It opens the
// engine on the first call that needs one and keeps it until Close, so a
// server with only desktop toolsets never opens hive.db. A failed open is
// tried again on the next call.
//
// The handlers are mounted on an in-memory MCP server, so a call is
// validated and answered exactly as the desktop's mount of the same table
// answers it.
type localEngine struct {
	// ctx outlives every call: the runtime's background work runs on it.
	ctx     context.Context
	open    engineOpener
	version string
	log     zerolog.Logger

	mu      sync.Mutex
	session *mcp.ClientSession
	close   func() error
}

func newLocalEngine(ctx context.Context, open engineOpener, version string, log zerolog.Logger) *localEngine {
	return &localEngine{ctx: ctx, open: open, version: version, log: log}
}

// Call runs one engine tool. An engine that does not open is an *ops.Error
// of kind unavailable.
func (e *localEngine) Call(ctx context.Context, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	cs, err := e.connect()
	if err != nil {
		return nil, err
	}
	return cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
}

func (e *localEngine) connect() (*mcp.ClientSession, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		return e.session, nil
	}

	handlers, closeEngine, err := e.open(e.ctx)
	if err != nil {
		e.log.Warn().Err(err).Msg("open the hive engine")
		return nil, ops.Wrap(ops.KindUnavailable, err, "open the hive engine: %v", err)
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "hive", Version: e.version}, nil)
	for _, op := range handlers.Ops() {
		if runsInProcess(op.Spec.Info()) {
			op.Mount(srv, e.internalError)
		}
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(e.ctx, st, nil)
	if err == nil {
		var cs *mcp.ClientSession
		cs, err = mcp.NewClient(&mcp.Implementation{Name: "hive", Version: e.version}, nil).Connect(e.ctx, ct, nil)
		if err == nil {
			e.session = cs
			e.close = func() error {
				return errors.Join(cs.Close(), closeEngine())
			}
			return cs, nil
		}
		_ = ss.Close()
	}
	return nil, errors.Join(err, closeEngine())
}

// internalError answers an error the handlers did not classify. Its text is
// for the log, as on the desktop.
func (e *localEngine) internalError(err error) error {
	e.log.Error().Err(err).Msg("engine tool failed")
	return ops.Errorf(ops.KindInternal, "hive could not complete the request")
}

// Close closes the engine if a call opened it.
func (e *localEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.close == nil {
		return nil
	}
	err := e.close()
	e.session, e.close = nil, nil
	return err
}
