package mcpsrv

import (
	"context"
	"time"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/ops/sessions"
)

type sessionView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Repo      string    `json:"repo"`
	State     string    `json:"state"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"createdAt"`
}

type listSessionsResult struct {
	Sessions []sessionView `json:"sessions"`
}

func sessionViewOf(s session.Session) sessionView {
	return sessionView{
		ID: s.ID, Name: s.Name, Slug: s.Slug, Repo: s.Remote, State: string(s.State),
		Tags: append([]string{}, s.Tags...), CreatedAt: s.CreatedAt,
	}
}

// ListSessions reports hive sessions, newest first, filtered by tags and
// state.
func (ctrl *Controller) ListSessions(ctx context.Context, in sessions.SessionListIn) (any, error) {
	sessions, err := ctrl.core.Sessions.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	views := []sessionView{}
	for _, s := range sessions {
		if in.State != "" && string(s.State) != in.State {
			continue
		}
		if !s.HasAllTags(in.Tags) {
			continue
		}
		views = append(views, sessionViewOf(s))
	}
	return listSessionsResult{Sessions: views}, nil
}

type editSessionTagsResult struct {
	Session sessionView `json:"session"`
}

// EditSessionTags adds and removes tags on one session. It changes the
// session record only; nothing is spawned or attached.
func (ctrl *Controller) EditSessionTags(ctx context.Context, in sessions.SessionTagsUpdateIn) (any, error) {
	saved, err := ctrl.core.Sessions.EditSessionTags(ctx, in.SessionID, in.Add, in.Remove)
	if err != nil {
		return nil, err
	}
	return editSessionTagsResult{Session: sessionViewOf(saved)}, nil
}
