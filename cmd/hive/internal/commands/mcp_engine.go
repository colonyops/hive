package commands

import (
	"context"
	"errors"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/hive/engineops"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
)

// openedEngine is what an engineOpener built: the handlers, the caller
// resolution of their engine, and the function that closes them.
type openedEngine struct {
	handlers *engineops.Handlers
	resolve  func(ctx context.Context, caller string) (sessionsvc.Caller, error)
	close    func() error
}

type engineOpener func(ctx context.Context) (*openedEngine, error)

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
	opened  *openedEngine
}

func newLocalEngine(ctx context.Context, open engineOpener, version string, log zerolog.Logger) *localEngine {
	return &localEngine{ctx: ctx, open: open, version: version, log: log}
}

// Call runs one engine tool. An engine that does not open is an *ops.Error
// of kind unavailable.
func (e *localEngine) Call(ctx context.Context, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	cs, _, err := e.connect()
	if err != nil {
		return nil, err
	}
	return cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
}

// IsChat reports whether caller is a chat. Chat records live in the
// desktop's store, so only the running desktop can file a chat's documents,
// as hive drive does.
func (e *localEngine) IsChat(ctx context.Context, caller string) (bool, error) {
	_, opened, err := e.connect()
	if err != nil {
		return false, err
	}
	c, err := opened.resolve(ctx, caller)
	return err == nil && c.Kind == sessionsvc.CallerKindChat, nil
}

func (e *localEngine) connect() (*mcp.ClientSession, *openedEngine, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		return e.session, e.opened, nil
	}

	opened, err := e.open(e.ctx)
	if err != nil {
		e.log.Warn().Err(err).Msg("open the hive engine")
		return nil, nil, ops.Wrap(ops.KindUnavailable, err, "open the hive engine: %v", err)
	}
	cs, err := e.mount(opened.handlers)
	if err != nil {
		e.log.Warn().Err(err).Msg("connect to the in-process engine server")
		return nil, nil, ops.Wrap(ops.KindUnavailable, err, "start the hive engine: %v", errors.Join(err, opened.close()))
	}
	e.session, e.opened = cs, opened
	return cs, opened, nil
}

func (e *localEngine) mount(handlers *engineops.Handlers) (*mcp.ClientSession, error) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "hive", Version: e.version}, nil)
	for _, op := range handlers.Ops() {
		if runsInProcess(op.Spec.Info()) {
			op.Mount(srv, e.internalError)
		}
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(e.ctx, st, nil)
	if err != nil {
		return nil, err
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "hive", Version: e.version}, nil).Connect(e.ctx, ct, nil)
	if err != nil {
		return nil, errors.Join(err, ss.Close())
	}
	return cs, nil
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
	if e.session == nil {
		return nil
	}
	err := errors.Join(e.session.Close(), e.opened.close())
	e.session, e.opened = nil, nil
	return err
}
