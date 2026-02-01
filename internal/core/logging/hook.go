package logging

import (
	"context"

	"github.com/rs/zerolog"
)

// ContextHook extracts session_id and agent_id from context and adds them to log events.
type ContextHook struct{}

// Run adds contextual fields to the zerolog event.
func (h ContextHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	ctx := e.GetCtx()
	if ctx == context.Background() || ctx == nil {
		return
	}

	if sessionID := GetSessionID(ctx); sessionID != "" {
		e.Str("session_id", sessionID)
	}

	if agentID := GetAgentID(ctx); agentID != "" {
		e.Str("agent_id", agentID)
	}
}
