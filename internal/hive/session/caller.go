package session

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/ops"
)

// CallerGlobal is the explicit caller value of an agent outside every session.
const CallerGlobal = "global"

// AgentSessionEnv names the environment variable that carries the caller: a
// hive session's id in a Code session, a chat's id in a chat.
const AgentSessionEnv = "HIVE_AGENT_SESSION"

const localServer = "local"

// CallerKind is what a caller value stands for.
//
// ENUM(session, chat, global)
type CallerKind string

// Caller is who a call acts for.
type Caller struct {
	Kind    CallerKind
	Session session.Session // Kind == CallerKindSession
	// ChatID is set for CallerKindChat. Chat records live in the desktop's
	// store, so the engine only says that the value is a chat id.
	ChatID string
}

// ResolveCaller answers who a call acts for. explicit is a --caller value or an
// operation's caller argument, env is HIVE_AGENT_SESSION and cwd is the working
// directory. The first non-empty one decides; a later one never replaces an
// earlier one that fails. A working directory inside no session is not_found:
// global is only ever an explicit value.
//
// Errors are *ops.Error.
func (s *Service) ResolveCaller(ctx context.Context, explicit, env, cwd string) (Caller, error) {
	explicit, env, cwd = strings.TrimSpace(explicit), strings.TrimSpace(env), strings.TrimSpace(cwd)
	switch {
	case explicit != "":
		return s.resolveCallerValue(ctx, explicit)
	case env != "":
		return s.resolveCallerValue(ctx, env)
	case cwd != "":
		return s.callerAtPath(ctx, cwd)
	}
	return Caller{}, ops.Errorf(ops.KindInvalid,
		"no caller: pass --caller, run inside a hive session, or set %s", AgentSessionEnv)
}

func (s *Service) resolveCallerValue(ctx context.Context, value string) (Caller, error) {
	switch {
	case value == CallerGlobal:
		return Caller{Kind: CallerKindGlobal}, nil
	case isChatUUID(value):
		return Caller{Kind: CallerKindChat, ChatID: value}, nil
	case isLegacyChatAlias(value):
		// A session id is six random base36 characters, so it can be all
		// digits: a session with this id wins over the chat alias.
		sess, err := s.sessions.Get(ctx, value)
		if err == nil {
			return Caller{Kind: CallerKindSession, Session: sess}, nil
		}
		if !errors.Is(err, session.ErrNotFound) {
			return Caller{}, ops.Errorf(ops.KindInternal, "load session %q: %v", value, err)
		}
		return Caller{Kind: CallerKindChat, ChatID: value}, nil
	case filepath.IsAbs(value):
		return s.callerAtPath(ctx, value)
	}

	id := value
	if server, rest, found := strings.Cut(value, ":"); found {
		if server != localServer {
			return Caller{}, ops.Errorf(ops.KindInvalid,
				"caller %q names server %q: the caller runs on this machine", value, server)
		}
		id = rest
	}
	if id == "" || strings.ContainsAny(id, ":/\\ \t") {
		return Caller{}, ops.Errorf(ops.KindInvalid,
			"caller %q is not a session id, a chat id, an absolute path, or %s", value, CallerGlobal)
	}

	sess, err := s.sessions.Get(ctx, id)
	if errors.Is(err, session.ErrNotFound) {
		return Caller{}, ops.Errorf(ops.KindNotFound, "session %q not found", id)
	}
	if err != nil {
		return Caller{}, ops.Errorf(ops.KindInternal, "load session %q: %v", id, err)
	}
	return Caller{Kind: CallerKindSession, Session: sess}, nil
}

func (s *Service) callerAtPath(ctx context.Context, path string) (Caller, error) {
	sess, ok, err := s.SessionAtPath(ctx, path)
	if err != nil {
		return Caller{}, ops.Errorf(ops.KindInternal, "find the hive session at %q: %v", path, err)
	}
	if !ok {
		return Caller{}, ops.Errorf(ops.KindNotFound,
			"no hive session's checkout holds %q; pass --caller <session id>, run inside a hive session, or pass %s outside both",
			path, CallerGlobal)
	}
	return Caller{Kind: CallerKindSession, Session: sess}, nil
}

// LooksLikeChatID reports whether value has the form of a chat id. It needs no
// store, for a program that runs without the engine and has no sessions to
// prefer.
func LooksLikeChatID(value string) bool {
	return isChatUUID(value) || isLegacyChatAlias(value)
}

func isChatUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

// isLegacyChatAlias reports whether value is a migrated chat's positive-integer
// alias.
func isLegacyChatAlias(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0
}
