package terminal_test

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
)

func TestMapEventToStatus(t *testing.T) {
	tests := []struct {
		event      string
		wantStatus terminal.Status
		wantOK     bool
	}{
		{"SessionStart", terminal.StatusReady, true},
		{"Stop", terminal.StatusReady, true},
		{"UserPromptSubmit", terminal.StatusActive, true},
		{"PermissionRequest", terminal.StatusApproval, true},
		{"Notification", terminal.StatusApproval, true},
		{"SessionEnd", terminal.StatusMissing, true},
		{"UnknownEvent", "", false},
		{"", "", false},
		{"PreCompact", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			got, ok := terminal.MapEventToStatus(tt.event)
			if ok != tt.wantOK {
				t.Errorf("MapEventToStatus(%q) ok = %v, want %v", tt.event, ok, tt.wantOK)
			}
			if got != tt.wantStatus {
				t.Errorf("MapEventToStatus(%q) status = %q, want %q", tt.event, got, tt.wantStatus)
			}
		})
	}
}

func TestHookResolverEnvVar(t *testing.T) {
	t.Setenv("HIVE_SESSION_ID", "test-session-id")
	t.Setenv("TMUX", "") // not inside tmux

	resolver := terminal.NewHookResolver(&stubSessionStore{})
	sessionID, windowIndex, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID != "test-session-id" {
		t.Errorf("sessionID = %q, want %q", sessionID, "test-session-id")
	}
	if windowIndex != "0" {
		t.Errorf("windowIndex = %q, want %q (not in tmux)", windowIndex, "0")
	}
}

func TestHookResolverCWDFallback(t *testing.T) {
	t.Setenv("HIVE_SESSION_ID", "")
	t.Setenv("TMUX", "") // not inside tmux

	// CWD won't match any session path → empty string returned, no error.
	resolver := terminal.NewHookResolver(&stubSessionStore{sessions: []session.Session{}})
	sessionID, windowIndex, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID != "" {
		t.Errorf("sessionID = %q, want empty (no matching session)", sessionID)
	}
	if windowIndex != "0" {
		t.Errorf("windowIndex = %q, want %q", windowIndex, "0")
	}
}

// stubSessionStore implements session.Store for testing.
type stubSessionStore struct {
	sessions []session.Session
}

func (s *stubSessionStore) List(_ context.Context) ([]session.Session, error) {
	return s.sessions, nil
}

func (s *stubSessionStore) Get(_ context.Context, _ string) (session.Session, error) {
	return session.Session{}, nil
}

func (s *stubSessionStore) Save(_ context.Context, _ session.Session) error { return nil }
func (s *stubSessionStore) Delete(_ context.Context, _ string) error        { return nil }
