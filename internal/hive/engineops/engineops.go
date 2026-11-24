// Package engineops serves the engine-owned operations of the catalog over
// *hive.Engine. One table backs every surface: the desktop mounts it on its
// MCP server and the hive CLI calls it in-process.
package engineops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/tagset"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
	driveops "github.com/colonyops/hive/internal/ops/drive"
	"github.com/colonyops/hive/internal/ops/mcpops"
	"github.com/colonyops/hive/internal/ops/messages"
	"github.com/colonyops/hive/internal/ops/sessions"
	"github.com/colonyops/hive/internal/ops/tasks"
	"github.com/colonyops/hive/internal/ops/todos"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/logutils"
)

// Handlers runs the engine operations. Every error it returns is an
// *ops.Error.
type Handlers struct {
	engine   *hive.Engine
	log      zerolog.Logger
	fleet    func() *servers.Fleet
	progress io.Writer
}

// Option changes how New builds the handlers.
type Option func(*Handlers)

// WithFleet routes the session operations through the fleet fleet returns,
// read on every call because a client rebuilds its fleet on a config reload.
// Without it they reach this engine only, as session.LocalServer.
func WithFleet(fleet func() *servers.Fleet) Option {
	return func(h *Handlers) { h.fleet = fleet }
}

// WithProgress receives the progress lines of a session create or recycle.
func WithProgress(w io.Writer) Option {
	return func(h *Handlers) {
		if w != nil {
			h.progress = w
		}
	}
}

func New(log zerolog.Logger, engine *hive.Engine, opts ...Option) *Handlers {
	h := &Handlers{engine: engine, log: logutils.Component(log, "engineops"), progress: io.Discard}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Handlers) currentFleet() *servers.Fleet {
	if h.fleet == nil {
		return nil
	}
	return h.fleet()
}

// Op is one engine operation bound to its handler.
type Op struct {
	Spec ops.Op
	// Call runs the handler on arguments a surface has already checked
	// against the spec, as the CLI does.
	Call func(ctx context.Context, args map[string]any) (any, error)
	// Mount registers the operation as a tool on srv.
	Mount func(srv *mcp.Server, mapErr func(error) error)
}

// Ops lists every engine operation of the catalog with its handler.
func (h *Handlers) Ops() []Op {
	return []Op{
		bind(h, tasks.TaskCreate, h.TaskCreate), bind(h, tasks.TaskList, h.TaskList), bind(h, tasks.TaskGet, h.TaskGet),
		bind(h, tasks.TaskUpdate, h.TaskUpdate), bind(h, tasks.TaskNext, h.TaskNext),
		bind(h, tasks.TaskCommentCreate, h.TaskCommentCreate), bind(h, tasks.TaskContextGet, h.TaskContextGet),
		bind(h, tasks.TaskClear, h.TaskClear),
		bind(h, messages.MessageCreate, h.MessageCreate), bind(h, messages.MessageList, h.MessageList),
		bind(h, messages.MessageReadPut, h.MessageReadPut), bind(h, messages.TopicList, h.TopicList),
		bind(h, todos.TodoCreate, h.TodoCreate), bind(h, todos.TodoList, h.TodoList), bind(h, todos.TodoUpdate, h.TodoUpdate),
		bind(h, driveops.DocumentList, h.DocumentList), bind(h, driveops.DocumentGet, h.DocumentGet),
		bind(h, driveops.DocumentPut, h.DocumentPut), bind(h, driveops.DocumentMove, h.DocumentMove),
		bind(h, driveops.DocumentDelete, h.DocumentDelete),
		bind(h, sessions.SessionList, h.SessionList), bind(h, sessions.SessionGet, h.SessionGet),
		bind(h, sessions.SessionUpdate, h.SessionUpdate), bind(h, sessions.SessionTagsUpdate, h.SessionTagsUpdate),
		bind(h, sessions.SessionDelete, h.SessionDelete), bind(h, sessions.SessionRecycle, h.SessionRecycle),
		bind(h, sessions.SessionCreate, h.SessionCreate), bind(h, sessions.SessionStart, h.SessionStart),
	}
}

func bind[In, Out any](handlers *Handlers, s ops.Spec[In], h func(context.Context, In) (Out, error)) Op {
	if handlers.engine == nil {
		h = func(context.Context, In) (Out, error) {
			var zero Out
			return zero, ops.Errorf(ops.KindUnavailable, "the hive engine is not running")
		}
	}
	return Op{
		Spec: s,
		Call: func(ctx context.Context, args map[string]any) (any, error) {
			raw, err := json.Marshal(args)
			if err != nil {
				return nil, ops.Errorf(ops.KindInvalid, "encode arguments: %v", err)
			}
			var in In
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, ops.Errorf(ops.KindInvalid, "%v", err)
			}
			return h(ctx, in)
		},
		Mount: func(srv *mcp.Server, mapErr func(error) error) { mcpops.Mount(srv, s, h, mapErr) },
	}
}

// caller is who an engine operation acts for. ID is the session's or the
// chat's id, and empty for a global caller. Dir is the directory a path
// caller named when no session holds it, so its repository can still scope
// the call.
type caller struct {
	ID      string
	Session *session.Session
	Dir     string
}

// resolve reads a caller value. A path inside no session is global with that
// directory, because the CLI sends its working directory when nothing else
// named a caller; an id that resolves to nothing stays not_found.
func (h *Handlers) resolve(ctx context.Context, value string) (caller, error) {
	c, err := h.engine.Sessions().ResolveCaller(ctx, value, "", "")
	if err != nil {
		if opErr, ok := errors.AsType[*ops.Error](err); ok && opErr.Kind == ops.KindNotFound && filepath.IsAbs(value) {
			return caller{Dir: value}, nil
		}
		return caller{}, err
	}
	switch c.Kind {
	case sessionsvc.CallerKindSession:
		return caller{ID: c.Session.ID, Session: &c.Session}, nil
	case sessionsvc.CallerKindChat:
		return caller{ID: c.ChatID}, nil
	default:
		return caller{}, nil
	}
}

// resolveAgent is resolve for a call that records who made it.
func (h *Handlers) resolveAgent(ctx context.Context, value, what string) (caller, error) {
	c, err := h.resolve(ctx, value)
	if err != nil {
		return caller{}, err
	}
	if c.ID == "" {
		return caller{}, ops.Errorf(ops.KindInvalid,
			"%s acts for a hive session or a chat, and the caller is neither; run it inside a hive session or pass --caller <session id>", what)
	}
	return c, nil
}

// repo is the repository a call is scoped to: explicit, else the caller
// session's remote, else the remote of the caller's directory.
func (h *Handlers) repo(ctx context.Context, explicit string, c caller) string {
	if explicit != "" {
		return explicit
	}
	remote := ""
	switch {
	case c.Session != nil:
		remote = c.Session.Remote
	case c.Dir != "":
		url, err := h.engine.Git().RemoteURL(ctx, c.Dir)
		if err != nil {
			h.log.Debug().Err(err).Str("dir", c.Dir).Msg("no git remote for the caller's directory")
			return ""
		}
		remote = url
	}
	owner, name := git.ExtractOwnerRepo(remote)
	if owner == "" || name == "" {
		return ""
	}
	return owner + "/" + name
}

// opError classifies an engine error. what names the failed step for the
// message.
func opError(err error, what string) error {
	if err == nil {
		return nil
	}
	if opErr, ok := errors.AsType[*ops.Error](err); ok {
		return opErr
	}
	kind := ops.KindInternal
	switch {
	case errors.Is(err, hc.ErrNotFound), errors.Is(err, messaging.ErrTopicNotFound),
		errors.Is(err, session.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		kind = ops.KindNotFound
	case errors.Is(err, hc.ErrCyclicDependency), errors.Is(err, session.ErrDuplicateName):
		kind = ops.KindConflict
	case errors.Is(err, hc.ErrInvalidTree), errors.Is(err, hc.ErrInvalidItemType), errors.Is(err, hc.ErrInvalidStatus),
		errors.Is(err, tagset.ErrInvalid), errors.Is(err, wire.ErrRemoteRequired):
		kind = ops.KindInvalid
	case errors.As(err, new(*wire.UnreachableError)):
		kind = ops.KindUnavailable
	}
	return ops.Errorf(kind, "%s: %v", what, err)
}
