package logging

import (
	"context"
	"testing"
)

func TestWithSessionID(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-123"

	ctx = WithSessionID(ctx, sessionID)
	got := GetSessionID(ctx)

	if got != sessionID {
		t.Errorf("GetSessionID() = %q, want %q", got, sessionID)
	}
}

func TestWithAgentID(t *testing.T) {
	ctx := context.Background()
	agentID := "test-agent-456"

	ctx = WithAgentID(ctx, agentID)
	got := GetAgentID(ctx)

	if got != agentID {
		t.Errorf("GetAgentID() = %q, want %q", got, agentID)
	}
}

func TestGetSessionID_NotPresent(t *testing.T) {
	ctx := context.Background()
	got := GetSessionID(ctx)

	if got != "" {
		t.Errorf("GetSessionID() = %q, want empty string", got)
	}
}

func TestGetAgentID_NotPresent(t *testing.T) {
	ctx := context.Background()
	got := GetAgentID(ctx)

	if got != "" {
		t.Errorf("GetAgentID() = %q, want empty string", got)
	}
}

func TestBothIDs(t *testing.T) {
	ctx := context.Background()
	sessionID := "session-1"
	agentID := "agent-1"

	ctx = WithSessionID(ctx, sessionID)
	ctx = WithAgentID(ctx, agentID)

	if got := GetSessionID(ctx); got != sessionID {
		t.Errorf("GetSessionID() = %q, want %q", got, sessionID)
	}

	if got := GetAgentID(ctx); got != agentID {
		t.Errorf("GetAgentID() = %q, want %q", got, agentID)
	}
}
