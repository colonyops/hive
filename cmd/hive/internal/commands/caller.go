package commands

import (
	"context"
	"errors"
	"os"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/pkg/logutils"
)

// resolveCallerOf asks the engine who is calling: explicit, else
// HIVE_AGENT_SESSION, else the working directory.
func resolveCallerOf(ctx context.Context, a *app.App, explicit string) (sessionsvc.Caller, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return sessionsvc.Caller{}, invalidf("find current directory: %w", err)
	}
	return a.Sessions().ResolveCaller(ctx, explicit, os.Getenv(sessionsvc.AgentSessionEnv), cwd)
}

// requireCallingSession returns the hive session a command acts for. A chat
// or global caller is invalid because the command records a hive session.
// notInSession replaces the engine's text when nothing named a caller and the
// working directory is inside no session.
func requireCallingSession(ctx context.Context, a *app.App, explicit, notInSession string) (session.Session, error) {
	caller, err := resolveCallerOf(ctx, a, explicit)
	if err != nil {
		if sess, ok := sessionByName(ctx, a, explicit); ok {
			return sess, nil
		}
		var opErr *ops.Error
		if errors.As(err, &opErr) {
			if opErr.Kind == ops.KindNotFound && explicit == "" && os.Getenv(sessionsvc.AgentSessionEnv) == "" {
				return session.Session{}, notFoundf("%s", notInSession)
			}
			return session.Session{}, &kindError{kind: opErr.Kind, err: errors.New(opErr.Msg)}
		}
		return session.Session{}, err
	}
	switch caller.Kind {
	case sessionsvc.CallerKindSession:
		return caller.Session, nil
	case sessionsvc.CallerKindChat:
		return session.Session{}, invalidf("this command acts for a hive session, and %s names a chat; run it from a hive session or pass --caller <session id>", sessionsvc.AgentSessionEnv)
	default:
		return session.Session{}, invalidf("this command acts for a hive session, not global; run it from a hive session")
	}
}

// callingSessionID returns the calling hive session's id, or "" when the
// caller is a chat, global, or in no session. Commands that only label their
// output with a session use it, so the reason for "" is logged and dropped.
func callingSessionID(ctx context.Context, a *app.App, explicit, component string) string {
	caller, err := resolveCallerOf(ctx, a, explicit)
	if err != nil {
		logger := logutils.Component(a.Logger, component)
		logger.Debug().Err(err).Msg("no calling session")
		return ""
	}
	return caller.Session.ID
}

// sessionByName keeps --caller <name> working: a name or slug is not a caller
// value the engine reads.
func sessionByName(ctx context.Context, a *app.App, ref string) (session.Session, bool) {
	if ref == "" {
		return session.Session{}, false
	}
	sessions, err := a.Sessions().ListSessions(ctx)
	if err != nil {
		return session.Session{}, false
	}
	for _, s := range sessions {
		if s.State == session.StateActive && (s.Name == ref || s.Slug == ref) {
			return s, true
		}
	}
	return session.Session{}, false
}
