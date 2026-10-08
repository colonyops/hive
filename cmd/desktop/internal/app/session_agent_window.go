package app

import (
	"context"
	"strings"

	"github.com/rs/zerolog"
)

type sessionAgentWindows interface {
	NewCommandWindow(ctx context.Context, slug, dir, name, command string) (string, error)
}

// NewAgentWindow starts a configured profile beside the session's existing
// windows, sharing its checkout without creating another Hive record.
func (s *SessionsService) NewAgentWindow(ctx context.Context, slug, agent string) (windowID string, resultErr error) {
	s.logger.Info().Str("session", slug).Str("agent", agent).Msg("agent window requested")
	defer func() {
		level := zerolog.InfoLevel
		if resultErr != nil {
			level = zerolog.ErrorLevel
		}
		s.logger.WithLevel(level).Err(resultErr).Str("session", slug).Str("agent", agent).Str("window_id", windowID).Msg("agent window request completed")
	}()
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return "", Errorf(KindInvalid, "agent profile is required")
	}
	if s.agentCommands == nil || s.agentWindows == nil {
		return "", Errorf(KindUnavailable, "agent windows are unavailable")
	}
	command, ok := s.agentCommands()[agent]
	if !ok || strings.TrimSpace(command) == "" {
		return "", Errorf(KindInvalid, "unknown agent profile %q", agent)
	}
	slug = strings.TrimSpace(slug)
	dir, err := s.SessionDirectory(ctx, slug)
	if err != nil {
		return "", err
	}
	id, err := s.agentWindows.NewCommandWindow(ctx, slug, dir, agent, command)
	if err != nil {
		return "", terminalError(err, "starting agent %q in session %q", agent, slug)
	}
	return id, nil
}
