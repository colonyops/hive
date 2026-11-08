package store

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/domain/project"
	"github.com/colonyops/hive/internal/store/db"
)

// ProjectStore implements project.Store using SQLite.
type ProjectStore struct {
	db *db.DB
}

var _ project.Store = (*ProjectStore)(nil)

func NewProjectStore(db *db.DB) *ProjectStore {
	return &ProjectStore{db: db}
}

func (s *ProjectStore) List(ctx context.Context) ([]project.Session, error) {
	rows, err := s.db.Ctx(ctx).ListProjectSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list project sessions: %w", err)
	}
	out := make([]project.Session, 0, len(rows))
	for _, row := range rows {
		ps, err := s.withRepos(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, ps)
	}
	return out, nil
}

func (s *ProjectStore) Get(ctx context.Context, id string) (project.Session, error) {
	row, err := s.db.Ctx(ctx).GetProjectSession(ctx, id)
	if IsNotFoundError(err) {
		return project.Session{}, project.ErrNotFound
	}
	if err != nil {
		return project.Session{}, fmt.Errorf("get project session: %w", err)
	}
	return s.withRepos(ctx, row)
}

func (s *ProjectStore) withRepos(ctx context.Context, row db.ProjectSession) (project.Session, error) {
	repos, err := s.db.Ctx(ctx).ListProjectSessionRepos(ctx, row.ID)
	if err != nil {
		return project.Session{}, fmt.Errorf("list project session repos: %w", err)
	}
	ps := project.Session{
		ID:        row.ID,
		Name:      row.Name,
		Slug:      row.Slug,
		Project:   row.Project,
		Root:      row.Root,
		Repos:     make([]project.Repo, 0, len(repos)),
		CreatedAt: time.Unix(0, row.CreatedAt),
		UpdatedAt: time.Unix(0, row.UpdatedAt),
	}
	for _, r := range repos {
		ps.Repos = append(ps.Repos, project.Repo{Dir: r.Dir, SessionID: r.SessionID})
	}
	return ps, nil
}

// Save writes the project session and replaces its repo list.
func (s *ProjectStore) Save(ctx context.Context, ps project.Session) error {
	return s.db.WithinTx(ctx, func(ctx context.Context, _ *db.DB) error {
		q := s.db.Ctx(ctx)
		err := q.UpsertProjectSession(ctx, db.UpsertProjectSessionParams{
			ID:        ps.ID,
			Name:      ps.Name,
			Slug:      ps.Slug,
			Project:   ps.Project,
			Root:      ps.Root,
			CreatedAt: ps.CreatedAt.UnixNano(),
			UpdatedAt: ps.UpdatedAt.UnixNano(),
		})
		if err != nil {
			return fmt.Errorf("save project session: %w", err)
		}
		if err := q.DeleteProjectSessionRepos(ctx, ps.ID); err != nil {
			return fmt.Errorf("clear project session repos: %w", err)
		}
		for i, r := range ps.Repos {
			err := q.InsertProjectSessionRepo(ctx, db.InsertProjectSessionRepoParams{
				ProjectSessionID: ps.ID,
				SessionID:        r.SessionID,
				Dir:              r.Dir,
				Position:         int64(i),
			})
			if err != nil {
				return fmt.Errorf("save project session repo %q: %w", r.Dir, err)
			}
		}
		return nil
	})
}

func (s *ProjectStore) Delete(ctx context.Context, id string) error {
	n, err := s.db.Ctx(ctx).DeleteProjectSession(ctx, id)
	if err != nil {
		return fmt.Errorf("delete project session: %w", err)
	}
	if n == 0 {
		return project.ErrNotFound
	}
	return nil
}

func (s *ProjectStore) ProjectOf(ctx context.Context, sessionID string) (project.Session, bool, error) {
	id, err := s.db.Ctx(ctx).ProjectSessionIDForSession(ctx, sessionID)
	if IsNotFoundError(err) {
		return project.Session{}, false, nil
	}
	if err != nil {
		return project.Session{}, false, fmt.Errorf("find project of session %q: %w", sessionID, err)
	}
	ps, err := s.Get(ctx, id)
	if err != nil {
		return project.Session{}, false, err
	}
	return ps, true, nil
}
