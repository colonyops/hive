// Package messaging provides inter-agent communication via pub/sub topics.
//
// # Inbox Convention
//
// Each hive session has a conventional inbox topic:
//
//	agent.<session-id>.inbox
//
// The "agent" prefix refers to the AI agent running in the session,
// not the session itself. This convention allows agents to discover
// each other's inboxes and send direct messages.
//
// # Future: Multi-Agent Addressing
//
// When hive supports multiple agents per session, inbox addressing will use:
//
//	agent.<session-id>.<agent-name>.inbox
//
// The current format will continue to work as an alias to the primary/default agent:
//
//	agent.<session-id>.inbox → agent.<session-id>.main.inbox
//
// Examples:
//
//	agent.26kj0c.inbox            // Current format (default agent)
//	agent.26kj0c.claude.inbox     // Future: Named agent "claude"
//	agent.26kj0c.test-runner.inbox // Future: Named agent "test-runner"
package messaging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hay-kot/hive/internal/core/session"
)

// SessionDetector finds the current session from the working directory.
type SessionDetector struct {
	store session.Store
}

// NewSessionDetector creates a new session detector.
func NewSessionDetector(store session.Store) *SessionDetector {
	return &SessionDetector{store: store}
}

// DetectSession returns the session ID for the current working directory.
// Returns empty string if not in a hive session, or an error if detection fails.
func (d *SessionDetector) DetectSession(ctx context.Context) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return d.DetectSessionFromPath(ctx, cwd)
}

// DetectSessionFromPath returns the session ID for the given path.
// Returns empty string if the path is not within a hive session, or an error if detection fails.
func (d *SessionDetector) DetectSessionFromPath(ctx context.Context, path string) (string, error) {
	sessions, err := d.store.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list sessions: %w", err)
	}

	// Clean and normalize the path
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("get absolute path: %w", err)
	}
	path = filepath.Clean(path)

	// Find the longest matching session path (most specific match)
	var bestMatch session.Session
	var bestMatchLen int

	for _, sess := range sessions {
		if sess.State != session.StateActive {
			continue
		}

		sessPath := filepath.Clean(sess.Path)

		// Check if path equals or is within the session path
		if path == sessPath || isSubpath(sessPath, path) {
			if len(sessPath) > bestMatchLen {
				bestMatch = sess
				bestMatchLen = len(sessPath)
			}
		}
	}

	return bestMatch.ID, nil
}

// isSubpath returns true if child is a subdirectory of parent.
func isSubpath(parent, child string) bool {
	// Ensure parent ends with separator for correct prefix matching
	if !strings.HasSuffix(parent, string(filepath.Separator)) {
		parent += string(filepath.Separator)
	}
	return strings.HasPrefix(child, parent)
}
