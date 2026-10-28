package command

import (
	"context"
	"fmt"
	"io"

	"github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

// Action is an alias for the unified action type.
type Action = action.Action

// SessionDeleter is the interface for deleting sessions.
type SessionDeleter interface {
	DeleteSession(ctx context.Context, id string) error
}

// SessionRecycler is the interface for recycling sessions.
type SessionRecycler interface {
	RecycleSession(ctx context.Context, id string, w io.Writer) error
}

// TmuxOpener opens or creates tmux sessions for hive sessions.
type TmuxOpener interface {
	OpenTmuxSession(ctx context.Context, name, path, remote, targetWindow string, background bool) (multiplexer.LaunchResult, error)
}

// WindowAdder adds windows to an existing tmux session.
type WindowAdder interface {
	AddWindowsToTmuxSession(ctx context.Context, tmuxName, workDir string, windows []multiplexer.WindowSpec, background bool) error
}

// SessionWindowCreator creates a Hive session, optionally runs shCmd in its
// directory, then opens windows in it. Non-zero shCmd exit aborts window
// creation.
type SessionWindowCreator interface {
	CreateSessionWithWindows(ctx context.Context, req sessionsvc.NewSessionRequest, windows []multiplexer.WindowSpec) (*session.Session, error)
}

// Deps are the collaborators executors call. Deleter, Recycler, Creator, and
// WindowCreator are the session API, which may run in a daemon; the rest run
// in the TUI's process. A nil field disables the actions that need it.
type Deps struct {
	Deleter       SessionDeleter
	Recycler      SessionRecycler
	Creator       SessionCreator
	WindowCreator SessionWindowCreator
	TmuxOpener    TmuxOpener
	WindowAdder   WindowAdder
	WindowKiller  WindowKiller
	// LocalRemote is the remote of the TUI's working directory. It fills an
	// empty remote on a new session, because the session API may run in a
	// process with another working directory.
	LocalRemote string
}

// Service creates command executors based on action type.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// NewCreateExecutor creates a CreateExecutor for streaming session creation.
func (s *Service) NewCreateExecutor(opts sessionsvc.CreateOptions) *CreateExecutor {
	if opts.Remote == "" {
		opts.Remote = s.deps.LocalRemote
	}
	return &CreateExecutor{
		creator: s.deps.Creator,
		opts:    opts,
	}
}

// CreateExecutor creates an executor for the given action.
// Returns error if the action type is not supported.
func (s *Service) CreateExecutor(a Action) (Executor, error) {
	switch a.Type {
	case action.TypeDelete:
		return &DeleteExecutor{
			deleter:   s.deps.Deleter,
			sessionID: a.SessionID,
		}, nil
	case action.TypeRecycle:
		return &RecycleExecutor{
			recycler:  s.deps.Recycler,
			sessionID: a.SessionID,
		}, nil
	case action.TypeShell:
		return &ShellExecutor{
			cmd: a.ShellCmd,
			dir: a.ShellDir,
		}, nil
	case action.TypeKillWindow:
		if a.WindowTarget == nil {
			return nil, fmt.Errorf("KillWindow action missing target")
		}
		if s.deps.WindowKiller == nil {
			return nil, fmt.Errorf("KillWindow service unavailable")
		}
		return &killWindowExecutor{killer: s.deps.WindowKiller, target: *a.WindowTarget}, nil
	case action.TypeSpawnWindows:
		if a.SpawnWindows == nil {
			return nil, fmt.Errorf("SpawnWindows action missing payload")
		}
		return &SpawnWindowsExecutor{
			payload:     a.SpawnWindows,
			adder:       s.deps.WindowAdder,
			creator:     s.deps.WindowCreator,
			localRemote: s.deps.LocalRemote,
		}, nil
	case action.TypeTmuxOpen:
		return &TmuxExecutor{
			opener:       s.deps.TmuxOpener,
			name:         a.SessionName,
			path:         a.SessionPath,
			remote:       a.SessionRemote,
			targetWindow: a.TmuxWindow,
			background:   false,
		}, nil
	case action.TypeTmuxStart:
		return &TmuxExecutor{
			opener:       s.deps.TmuxOpener,
			name:         a.SessionName,
			path:         a.SessionPath,
			remote:       a.SessionRemote,
			targetWindow: a.TmuxWindow,
			background:   true,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported action type: %s", a.Type)
	}
}
