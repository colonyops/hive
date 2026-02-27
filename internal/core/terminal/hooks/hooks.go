// Package hooks implements a terminal integration that reads agent status from
// files written by Claude Code hooks instead of parsing tmux terminal output.
package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
)

const (
	// StatusFileName is the file written by Claude Code hooks inside the session directory.
	StatusFileName = ".hive-agent-status"

	// sessionPathKey is the metadata key injected by the TUI carrying the session path.
	sessionPathKey = "_session_path"

	// maxActiveAge is the maximum age of an "active" status before it is considered stale.
	// Claude Code fires PreToolUse before every tool, so a gap longer than this means
	// the process likely crashed or was killed without firing the Stop hook.
	maxActiveAge = 60 * time.Second

	// maxReadyAge is the maximum age of a "ready" status before it is considered stale.
	maxReadyAge = 10 * time.Minute
)

// Integration implements terminal.Integration using status files written by Claude Code hooks.
// When Claude Code hooks are installed in a session directory, they write the current
// agent state ("active" or "ready") to StatusFileName. This integration reads that file
// to provide accurate status without parsing tmux terminal output.
type Integration struct{}

// New creates a new hooks integration.
func New() *Integration {
	return &Integration{}
}

// Name returns "hooks".
func (h *Integration) Name() string {
	return "hooks"
}

// Available always returns true — the integration relies on files, not external binaries.
func (h *Integration) Available() bool {
	return true
}

// RefreshCache is a no-op; status files are read on demand.
func (h *Integration) RefreshCache() {}

// DiscoverSession checks whether a hooks-written status file exists for the session.
// The session path is read from the "_session_path" metadata key injected by the TUI.
// Returns nil if the metadata is missing or the status file does not exist.
func (h *Integration) DiscoverSession(_ context.Context, slug string, metadata map[string]string) (*terminal.SessionInfo, error) {
	sessionPath := metadata[sessionPathKey]
	if sessionPath == "" {
		return nil, nil
	}

	statusFile := filepath.Join(sessionPath, StatusFileName)
	if _, err := os.Stat(statusFile); err != nil {
		return nil, nil
	}

	// Use Pane to carry the session path through to GetStatus.
	return &terminal.SessionInfo{
		Name: slug,
		Pane: sessionPath,
	}, nil
}

// GetStatus reads the status file written by Claude Code hooks and returns the
// corresponding terminal status. Returns StatusMissing if the file is absent or stale.
func (h *Integration) GetStatus(_ context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	if info == nil {
		return terminal.StatusMissing, nil
	}

	sessionPath := info.Pane
	statusFile := filepath.Join(sessionPath, StatusFileName)

	fi, err := os.Stat(statusFile)
	if err != nil {
		return terminal.StatusMissing, nil
	}

	age := time.Since(fi.ModTime())

	raw, err := os.ReadFile(statusFile)
	if err != nil {
		return terminal.StatusMissing, nil
	}

	status := strings.TrimSpace(string(raw))

	switch terminal.Status(status) {
	case terminal.StatusActive:
		if age > maxActiveAge {
			return terminal.StatusMissing, nil
		}
		return terminal.StatusActive, nil
	case terminal.StatusApproval:
		return terminal.StatusApproval, nil
	case terminal.StatusReady:
		if age > maxReadyAge {
			return terminal.StatusMissing, nil
		}
		return terminal.StatusReady, nil
	default:
		return terminal.StatusMissing, nil
	}
}

// Ensure Integration satisfies the terminal.Integration interface at compile time.
var _ terminal.Integration = (*Integration)(nil)
