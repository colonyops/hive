package app

import (
	"context"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

// CreateChat is a chat start from outside the New chat dialog: an applet, an
// agent over MCP, or hive dt.
type CreateChat struct {
	Workspace string
	Name      string
	// Prompt is the opening message as the caller wrote it. Unlike a scheduled
	// chat's, it is not framed: the caller may mean a long-running chat.
	Prompt    string
	PinToCode bool
}

// CreateChat starts a detached chat, because the caller has no pane to
// attach. The chat attaches when someone opens it, as a scheduled one does.
func (s *AgentWorkspacesService) CreateChat(ctx context.Context, req CreateChat) (SessionView, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return SessionView{}, Errorf(KindInvalid, "a chat needs a name")
	}
	regen, err := s.regenerate(ctx, req.Workspace)
	if err != nil {
		return SessionView{}, err
	}
	view, err := s.startSession(ctx, regen.status.Workspace, StartSession{
		Workspace: req.Workspace, Name: name, Prompt: req.Prompt, Detached: true,
	})
	if err != nil {
		return SessionView{}, err
	}
	if view.ExitedEarly {
		if err := s.sessions.Delete(ctx, view.ID); err != nil {
			s.logger.Warn().Ctx(ctx).Err(err).Str("chat_id", view.ID).Msg("deleting a chat whose agent exited at launch")
		}
		return SessionView{}, Errorf(KindInternal, "%s", view.Notice)
	}
	if req.PinToCode {
		at := time.Now().UnixMilli()
		if _, err := s.sessions.SetPinnedAt(ctx, view.ID, at); err != nil {
			// The chat runs, so this is not a failed start: a retry would open
			// a second chat.
			s.logger.Warn().Ctx(ctx).Err(err).Str("chat_id", view.ID).Msg("pinning a new chat")
		} else {
			view.PinnedAt = at
		}
	}
	s.events.Publish(ctx, events.AgentChatsUpdated{})
	return view, nil
}

// ListChats lists every chat, or one workspace's when workspace is set.
func (s *AgentWorkspacesService) ListChats(ctx context.Context, workspace string) ([]SessionView, error) {
	if workspace == "" {
		return s.AllSessions(ctx)
	}
	if _, err := s.knownWorkspaceDir(workspace); err != nil {
		return nil, err
	}
	return s.Sessions(ctx, workspace)
}

// PinChat pins a chat to the Code sidebar, after the chats already pinned,
// or unpins it. A chat already in the asked state keeps its place.
func (s *AgentWorkspacesService) PinChat(ctx context.Context, id string, pinned bool) (SessionView, error) {
	rec, err := s.getSession(ctx, id)
	if err != nil {
		return SessionView{}, err
	}
	if pinned != (rec.PinnedAt > 0) {
		at := int64(0)
		if pinned {
			at = time.Now().UnixMilli()
		}
		if _, err := s.sessions.SetPinnedAt(ctx, rec.ID, at); err != nil {
			return SessionView{}, Wrap(err, KindInternal, "pinning chat %q", id)
		}
		rec.PinnedAt = at
		s.events.Publish(ctx, events.AgentChatsUpdated{})
	}
	return s.sessionViews(ctx, []stores.AgentSession{rec})[0], nil
}

// ImportPins pins ids in order, for the pins the sidebar kept in browser
// storage before the chat record held them. An id with no chat is skipped, as
// the sidebar skipped it, and a chat already pinned keeps its place.
func (s *AgentWorkspacesService) ImportPins(ctx context.Context, ids []string) error {
	base := time.Now().UnixMilli()
	changed := false
	for i, id := range ids {
		rec, err := s.sessions.Get(ctx, id)
		if stores.IsNotFound(err) {
			continue
		}
		if err != nil {
			return Wrap(err, KindInternal, "loading chat %q", id)
		}
		if rec.PinnedAt > 0 {
			continue
		}
		if _, err := s.sessions.SetPinnedAt(ctx, rec.ID, base+int64(i)); err != nil {
			return Wrap(err, KindInternal, "pinning chat %q", id)
		}
		changed = true
	}
	if changed {
		s.events.Publish(ctx, events.AgentChatsUpdated{})
	}
	return nil
}
