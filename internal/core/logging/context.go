package logging

import "context"

type contextKey string

const (
	sessionIDKey contextKey = "session_id"
	agentIDKey   contextKey = "agent_id"
)

// WithSessionID adds a session ID to the context.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// WithAgentID adds an agent ID to the context.
func WithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, agentIDKey, agentID)
}

// GetSessionID retrieves the session ID from the context.
// Returns empty string if not present.
func GetSessionID(ctx context.Context) string {
	if id, ok := ctx.Value(sessionIDKey).(string); ok {
		return id
	}
	return ""
}

// GetAgentID retrieves the agent ID from the context.
// Returns empty string if not present.
func GetAgentID(ctx context.Context) string {
	if id, ok := ctx.Value(agentIDKey).(string); ok {
		return id
	}
	return ""
}
