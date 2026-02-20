package command

import (
	"context"
	"fmt"
	"io"

	"github.com/colonyops/hive/internal/core/action"
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
	OpenTmuxSession(ctx context.Context, name, path, remote, targetWindow string, background bool) error
}

// WindowSpawner handles window operations for SpawnWindows actions.
type WindowSpawner interface {
	// AddWindowsToTmuxSession adds windows to an existing tmux session.
	AddWindowsToTmuxSession(ctx context.Context, tmuxName, workDir string, windows []action.WindowSpec, background bool) error
	// CreateSessionWithWindows creates a new Hive session, optionally runs shCmd in its directory,
	// then opens windows in it. Non-zero shCmd exit aborts window creation.
	CreateSessionWithWindows(ctx context.Context, req action.NewSessionRequest, windows []action.WindowSpec, background bool) error
}

// Service creates command executors based on action type.
type Service struct {
	deleter       SessionDeleter
	recycler      SessionRecycler
	tmuxOpener    TmuxOpener
	windowSpawner WindowSpawner
}

// NewService creates a new command service with the given dependencies.
func NewService(deleter SessionDeleter, recycler SessionRecycler, tmuxOpener TmuxOpener, windowSpawner WindowSpawner) *Service {
	return &Service{
		deleter:       deleter,
		recycler:      recycler,
		tmuxOpener:    tmuxOpener,
		windowSpawner: windowSpawner,
	}
}

// CreateExecutor creates an executor for the given action.
// Returns error if the action type is not supported.
func (s *Service) CreateExecutor(a Action) (Executor, error) {
	switch a.Type {
	case action.TypeDelete:
		return &DeleteExecutor{
			deleter:   s.deleter,
			sessionID: a.SessionID,
		}, nil
	case action.TypeRecycle:
		return &RecycleExecutor{
			recycler:  s.recycler,
			sessionID: a.SessionID,
		}, nil
	case action.TypeShell:
		return &ShellExecutor{
			cmd: a.ShellCmd,
			dir: a.ShellDir,
		}, nil
	case action.TypeSpawnWindows:
		if a.SpawnWindows == nil {
			return nil, fmt.Errorf("SpawnWindows action missing payload")
		}
		return &SpawnWindowsExecutor{
			payload: a.SpawnWindows,
			spawner: s.windowSpawner,
		}, nil
	case action.TypeTmuxOpen:
		return &TmuxExecutor{
			opener:       s.tmuxOpener,
			name:         a.SessionName,
			path:         a.SessionPath,
			remote:       a.SessionRemote,
			targetWindow: a.TmuxWindow,
			background:   false,
		}, nil
	case action.TypeTmuxStart:
		return &TmuxExecutor{
			opener:       s.tmuxOpener,
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
