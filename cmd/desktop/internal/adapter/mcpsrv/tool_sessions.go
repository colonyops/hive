package mcpsrv

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/domain/session"
)

type listSessionsInput struct {
	Labels []string `json:"labels,omitempty" jsonschema:"Only sessions carrying every one of these labels. Labels compare case-insensitively."`
	State  string   `json:"state,omitempty"  jsonschema:"Only sessions in this state: active, recycled or corrupted."`
}

type sessionView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Repo      string    `json:"repo"`
	State     string    `json:"state"`
	Labels    []string  `json:"labels"`
	CreatedAt time.Time `json:"createdAt"`
}

type listSessionsResult struct {
	Sessions []sessionView `json:"sessions"`
}

func sessionViewOf(s session.Session) sessionView {
	return sessionView{
		ID: s.ID, Name: s.Name, Slug: s.Slug, Repo: s.Remote, State: string(s.State),
		Labels: append([]string{}, s.Labels...), CreatedAt: s.CreatedAt,
	}
}

// ListSessions reports hive sessions, newest first, filtered by labels and
// state.
func (ctrl *Controller) ListSessions(ctx context.Context, _ *mcp.CallToolRequest, in listSessionsInput) (*mcp.CallToolResult, any, error) {
	switch session.State(in.State) {
	case "", session.StateActive, session.StateRecycled, session.StateCorrupted:
	default:
		return nil, nil, ctrl.toolError(app.Errorf(app.KindInvalid, "state %q is not active, recycled or corrupted", in.State))
	}
	sessions, err := ctrl.core.Sessions.ListSessions(ctx)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	views := []sessionView{}
	for _, s := range sessions {
		if in.State != "" && string(s.State) != in.State {
			continue
		}
		if !s.HasAllLabels(in.Labels) {
			continue
		}
		views = append(views, sessionViewOf(s))
	}
	return nil, listSessionsResult{Sessions: views}, nil
}

type editSessionLabelsInput struct {
	SessionID string   `json:"sessionId"        jsonschema:"Session id, from list_sessions."`
	Add       []string `json:"add,omitempty"    jsonschema:"Labels to add. A label the session already carries is kept as it is."`
	Remove    []string `json:"remove,omitempty" jsonschema:"Labels to remove, after the adds. A label the session does not carry is ignored."`
}

type editSessionLabelsResult struct {
	Session sessionView `json:"session"`
}

// EditSessionLabels adds and removes labels on one session. It changes the
// session record only; nothing is spawned or attached.
func (ctrl *Controller) EditSessionLabels(ctx context.Context, _ *mcp.CallToolRequest, in editSessionLabelsInput) (*mcp.CallToolResult, any, error) {
	if len(in.Add) == 0 && len(in.Remove) == 0 {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindInvalid, "provide add or remove"))
	}
	saved, err := ctrl.core.Sessions.EditSessionLabels(ctx, in.SessionID, in.Add, in.Remove)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	return nil, editSessionLabelsResult{Session: sessionViewOf(saved)}, nil
}
