package stores

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/pkg/randid"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type AgentSessionStore struct {
	q      *queries.DB
	now    func() time.Time
	mapper MapFunc[queries.AgentWorkspaceSession, AgentSession]
}

func NewAgentSessionStore(q *queries.DB, opts Options) *AgentSessionStore {
	return &AgentSessionStore{q: q, now: opts.Now, mapper: mapAgentSessionFromDB}
}

// Creation order stays stable when a session resumes.
func (s *AgentSessionStore) List(ctx context.Context, workspace string) ([]AgentSession, error) {
	rows, err := s.q.Ctx(ctx).ListAgentWorkspaceSessions(ctx, workspace)
	return s.mapper.SliceErr(rows, wrap("listing agent workspace sessions", err))
}

func (s *AgentSessionStore) ListAll(ctx context.Context) ([]AgentSession, error) {
	rows, err := s.q.Ctx(ctx).ListAllAgentWorkspaceSessions(ctx)
	return s.mapper.SliceErr(rows, wrap("listing all agent workspace sessions", err))
}

// A missing session returns NotFoundError.
func (s *AgentSessionStore) Get(ctx context.Context, id int64) (AgentSession, error) {
	row, err := s.q.Ctx(ctx).GetAgentWorkspaceSession(ctx, id)
	return s.mapper.Err(row, errTransformQueryOne("agent_workspace_session", fmt.Sprint(id), err))
}

func (s *AgentSessionStore) Create(ctx context.Context, in AgentSessionCreate) (AgentSession, error) {
	now := s.now().UnixMilli()
	if in.TerminalID == "" {
		in.TerminalID = randid.Generate(8)
	}
	row, err := s.q.Ctx(ctx).InsertAgentWorkspaceSession(ctx, queries.InsertAgentWorkspaceSessionParams{
		Workspace: in.Workspace, Name: in.Name, Agent: in.Agent, AgentSessionID: in.AgentSessionID,
		TerminalID: in.TerminalID, CreatedAt: now, LastOpenedAt: now, ScheduleID: in.ScheduleID, EndToken: in.EndToken,
	})
	return s.mapper.Err(row, wrap("creating agent workspace session", err))
}

// GetByEndToken resolves the session whose launch handed out token. An empty
// token is NotFoundError: a row from before the column existed must not
// answer for a blank bearer.
func (s *AgentSessionStore) GetByEndToken(ctx context.Context, token string) (AgentSession, error) {
	row, err := s.q.Ctx(ctx).GetAgentWorkspaceSessionByEndToken(ctx, token)
	return s.mapper.Err(row, errTransformQueryOne("agent_workspace_session", "token", err))
}

// LastOpenedAt controls session ordering.
func (s *AgentSessionStore) Touch(ctx context.Context, id, at int64) error {
	return wrap("touching agent workspace session", s.q.Ctx(ctx).TouchAgentWorkspaceSession(ctx, queries.TouchAgentWorkspaceSessionParams{
		LastOpenedAt: at,
		ID:           id,
	}))
}

// SetAgentID records the id a resume-that-fell-back-to-fresh now runs
// under, so a later resume of this same record addresses the conversation
// actually running rather than the one it replaced.
func (s *AgentSessionStore) SetAgentID(ctx context.Context, id int64, agentSessionID string) error {
	return wrap("setting agent workspace session agent id", s.q.Ctx(ctx).SetAgentWorkspaceSessionAgentID(ctx, queries.SetAgentWorkspaceSessionAgentIDParams{
		AgentSessionID: agentSessionID,
		ID:             id,
	}))
}

// Display names do not affect the immutable terminal id.
func (s *AgentSessionStore) Rename(ctx context.Context, id int64, name string) error {
	return wrap("renaming agent workspace session", s.q.Ctx(ctx).RenameAgentWorkspaceSession(ctx, queries.RenameAgentWorkspaceSessionParams{
		Name: name,
		ID:   id,
	}))
}

// Callers must close the live terminal first to avoid orphaning its agent.
func (s *AgentSessionStore) Delete(ctx context.Context, id int64) error {
	return wrap("deleting agent workspace session", s.q.Ctx(ctx).DeleteAgentWorkspaceSession(ctx, id))
}

// Workspace deletion removes the directory separately; this deletes session
// history only.
func (s *AgentSessionStore) DeleteByWorkspace(ctx context.Context, workspace string) error {
	return wrap("deleting agent workspace sessions by workspace", s.q.Ctx(ctx).DeleteAgentWorkspaceSessionsByWorkspace(ctx, workspace))
}
