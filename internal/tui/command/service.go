package command

import (
	"context"
	"fmt"
	"io"
)

// SessionDeleter is the interface for deleting sessions.
type SessionDeleter interface {
	DeleteSession(ctx context.Context, id string) error
}

// SessionRecycler is the interface for recycling sessions.
type SessionRecycler interface {
	RecycleSession(ctx context.Context, id string, w io.Writer) error
}

// Service creates command executors based on action type.
type Service struct {
	deleter  SessionDeleter
	recycler SessionRecycler
}

// NewService creates a new command service with the given dependencies.
func NewService(deleter SessionDeleter, recycler SessionRecycler) *Service {
	return &Service{
		deleter:  deleter,
		recycler: recycler,
	}
}

// ActionType identifies the kind of action.
// ENUM(none, recycle, delete, shell).
type ActionType string

// Action represents a resolved command action ready for execution.
type Action struct {
	Type        ActionType
	Key         string
	Help        string
	Confirm     string // Non-empty if confirmation required
	ShellCmd    string // For shell actions, the rendered command
	SessionID   string
	SessionPath string
	Silent      bool  // Skip loading popup for fast commands
	Exit        bool  // Exit hive after command completes
	Err         error // Non-nil if action resolution failed (e.g., template error)
}

// NeedsConfirm returns true if the action requires user confirmation.
func (a Action) NeedsConfirm() bool {
	return a.Confirm != ""
}

// CreateExecutor creates an executor for the given action.
// Returns error if the action type is not supported.
func (s *Service) CreateExecutor(action Action) (Executor, error) {
	switch action.Type {
	case ActionTypeDelete:
		return &DeleteExecutor{
			deleter:   s.deleter,
			sessionID: action.SessionID,
		}, nil
	case ActionTypeRecycle:
		return &RecycleExecutor{
			recycler:  s.recycler,
			sessionID: action.SessionID,
		}, nil
	case ActionTypeShell:
		return &ShellExecutor{
			cmd: action.ShellCmd,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported action type: %s", action.Type)
	}
}
