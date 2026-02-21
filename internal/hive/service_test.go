package hive

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements session.Store for testing.
type mockStore struct {
	sessions map[string]session.Session
}

func newMockStore() *mockStore {
	return &mockStore{sessions: make(map[string]session.Session)}
}

func (m *mockStore) List(_ context.Context) ([]session.Session, error) {
	var result []session.Session
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result, nil
}

func (m *mockStore) Get(_ context.Context, id string) (session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	return s, nil
}

func (m *mockStore) Save(_ context.Context, s session.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockStore) Delete(_ context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockStore) FindRecyclable(_ context.Context, remote string) (session.Session, error) {
	for _, s := range m.sessions {
		if s.State == session.StateRecycled && s.Remote == remote {
			return s, nil
		}
	}
	return session.Session{}, session.ErrNoRecyclable
}

// mockExec implements executil.Executor for testing.
type mockExec struct{}

func (m *mockExec) Run(context.Context, string, ...string) ([]byte, error) { return nil, nil }

func (m *mockExec) RunDir(context.Context, string, string, ...string) ([]byte, error) {
	return nil, nil
}

func (m *mockExec) RunStream(context.Context, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func (m *mockExec) RunDirStream(context.Context, string, io.Writer, io.Writer, string, ...string) error {
	return nil
}

// mockGit implements git.Git for testing.
type mockGit struct{}

func (m *mockGit) Clone(_ context.Context, _, _ string) error            { return nil }
func (m *mockGit) Checkout(_ context.Context, _, _ string) error         { return nil }
func (m *mockGit) Pull(_ context.Context, _ string) error                { return nil }
func (m *mockGit) ResetHard(_ context.Context, _ string) error           { return nil }
func (m *mockGit) RemoteURL(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockGit) IsClean(_ context.Context, _ string) (bool, error)     { return true, nil }
func (m *mockGit) Branch(_ context.Context, _ string) (string, error)    { return "main", nil }
func (m *mockGit) DefaultBranch(_ context.Context, _ string) (string, error) {
	return "main", nil
}
func (m *mockGit) DiffStats(_ context.Context, _ string) (int, int, error) { return 0, 0, nil }
func (m *mockGit) IsValidRepo(_ context.Context, _ string) error           { return nil }

func newTestService(t *testing.T, store session.Store, cfg *config.Config) *SessionService {
	t.Helper()
	return newTestServiceWithBus(t, store, cfg, testbus.New(t).EventBus)
}

func newTestServiceWithBus(t *testing.T, store session.Store, cfg *config.Config, bus *eventbus.EventBus) *SessionService {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
		}
	}
	log := zerolog.New(io.Discard)
	renderer := tmpl.New(tmpl.Config{})
	return NewSessionService(store, &mockGit{}, cfg, bus, &mockExec{}, renderer, log, io.Discard, io.Discard)
}

func TestRenameSession(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	past := time.Now().Add(-1 * time.Hour)
	sess := session.Session{
		ID:        "test1",
		Name:      "old-name",
		Slug:      "old-name",
		State:     session.StateActive,
		CreatedAt: past,
		UpdatedAt: past,
	}
	require.NoError(t, store.Save(context.Background(), sess))

	err := svc.RenameSession(context.Background(), "test1", "new-name")
	require.NoError(t, err)

	updated, err := store.Get(context.Background(), "test1")
	require.NoError(t, err)
	assert.Equal(t, "new-name", updated.Name)
	assert.Equal(t, "new-name", updated.Slug)
	assert.True(t, updated.UpdatedAt.After(past))
}

func TestRenameSession_Slugify(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	sess := session.Session{
		ID:    "test1",
		Name:  "old",
		Slug:  "old",
		State: session.StateActive,
	}
	require.NoError(t, store.Save(context.Background(), sess))

	err := svc.RenameSession(context.Background(), "test1", "My Feature Branch!")
	require.NoError(t, err)

	updated, err := store.Get(context.Background(), "test1")
	require.NoError(t, err)
	assert.Equal(t, "My Feature Branch!", updated.Name)
	assert.Equal(t, "my-feature-branch", updated.Slug)
}

func TestRenameSession_NotFound(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	err := svc.RenameSession(context.Background(), "nonexistent", "new-name")
	assert.Error(t, err)
}

func TestRenameSession_EmptyName(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	err := svc.RenameSession(context.Background(), "test1", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name cannot be empty")
}

func TestRenameSession_EmptySlug(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	err := svc.RenameSession(context.Background(), "test1", "!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty slug")
}

func TestEnforceMaxRecycled(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	t.Run("unlimited (0) does nothing", func(t *testing.T) {
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules:   []config.Rule{{Pattern: "", MaxRecycled: intPtr(0)}},
		}
		svc := newTestService(t, store, cfg)

		// Add sessions
		remote := "https://github.com/test/repo"
		for i := 0; i < 10; i++ {
			sess := session.Session{
				ID:        string(rune('a' + i)),
				Remote:    remote,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: time.Now().Add(time.Duration(i) * time.Hour),
			}
			store.sessions[sess.ID] = sess
		}

		err := svc.enforceMaxRecycled(context.Background(), remote)
		require.NoError(t, err)

		// All 10 should still exist
		sessions, _ := store.List(context.Background())
		assert.Len(t, sessions, 10)
	})

	t.Run("deletes oldest beyond limit", func(t *testing.T) {
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules:   []config.Rule{{Pattern: "", MaxRecycled: intPtr(3)}},
		}
		svc := newTestService(t, store, cfg)

		remote := "https://github.com/test/repo"
		baseTime := time.Now()

		// Create 5 sessions with different ages
		for i := 0; i < 5; i++ {
			sess := session.Session{
				ID:        string(rune('a' + i)),
				Remote:    remote,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: baseTime.Add(time.Duration(i) * time.Hour), // a=oldest, e=newest
			}
			store.sessions[sess.ID] = sess
		}

		err := svc.enforceMaxRecycled(context.Background(), remote)
		require.NoError(t, err)

		// Should keep 3 newest: c, d, e
		sessions, _ := store.List(context.Background())
		assert.Len(t, sessions, 3)

		// Verify the newest 3 remain
		for _, s := range sessions {
			assert.Contains(t, []string{"c", "d", "e"}, s.ID, "expected only newest sessions to remain")
		}
	})

	t.Run("only affects matching remote", func(t *testing.T) {
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules:   []config.Rule{{Pattern: "", MaxRecycled: intPtr(1)}},
		}
		svc := newTestService(t, store, cfg)

		remote1 := "https://github.com/test/repo1"
		remote2 := "https://github.com/test/repo2"
		baseTime := time.Now()

		// 3 sessions for repo1
		for i := 0; i < 3; i++ {
			sess := session.Session{
				ID:        string(rune('a' + i)),
				Remote:    remote1,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: baseTime.Add(time.Duration(i) * time.Hour),
			}
			store.sessions[sess.ID] = sess
		}

		// 3 sessions for repo2
		for i := 0; i < 3; i++ {
			sess := session.Session{
				ID:        string(rune('x' + i)),
				Remote:    remote2,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: baseTime.Add(time.Duration(i) * time.Hour),
			}
			store.sessions[sess.ID] = sess
		}

		// Enforce only for repo1
		err := svc.enforceMaxRecycled(context.Background(), remote1)
		require.NoError(t, err)

		sessions, _ := store.List(context.Background())
		// 1 from repo1 + 3 from repo2 = 4
		assert.Len(t, sessions, 4)
	})
}

func TestPrune(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	t.Run("all=true deletes all recycled", func(t *testing.T) {
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules:   []config.Rule{{Pattern: "", MaxRecycled: intPtr(10)}}, // high limit shouldn't matter with all=true
		}
		svc := newTestService(t, store, cfg)

		remote := "https://github.com/test/repo"
		// Add recycled sessions
		for i := 0; i < 5; i++ {
			sess := session.Session{
				ID:        string(rune('a' + i)),
				Remote:    remote,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: time.Now(),
			}
			store.sessions[sess.ID] = sess
		}
		// Add active session
		store.sessions["active"] = session.Session{
			ID:     "active",
			Remote: remote,
			State:  session.StateActive,
			Path:   t.TempDir(),
		}

		count, err := svc.Prune(context.Background(), true)
		require.NoError(t, err)
		assert.Equal(t, 5, count)

		sessions, _ := store.List(context.Background())
		assert.Len(t, sessions, 1)
		assert.Equal(t, "active", sessions[0].ID)
	})

	t.Run("all=false respects max_recycled", func(t *testing.T) {
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules:   []config.Rule{{Pattern: "", MaxRecycled: intPtr(2)}},
		}
		svc := newTestService(t, store, cfg)

		remote := "https://github.com/test/repo"
		baseTime := time.Now()

		// Add 5 recycled sessions
		for i := 0; i < 5; i++ {
			sess := session.Session{
				ID:        string(rune('a' + i)),
				Remote:    remote,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: baseTime.Add(time.Duration(i) * time.Hour),
			}
			store.sessions[sess.ID] = sess
		}

		count, err := svc.Prune(context.Background(), false)
		require.NoError(t, err)
		assert.Equal(t, 3, count) // Should delete 3 (5-2)

		sessions, _ := store.List(context.Background())
		assert.Len(t, sessions, 2)
	})

	t.Run("always deletes corrupted", func(t *testing.T) {
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules:   []config.Rule{{Pattern: "", MaxRecycled: intPtr(10)}}, // high limit
		}
		svc := newTestService(t, store, cfg)

		// Add corrupted session
		store.sessions["corrupted"] = session.Session{
			ID:    "corrupted",
			State: session.StateCorrupted,
			Path:  t.TempDir(),
		}
		// Add active session
		store.sessions["active"] = session.Session{
			ID:    "active",
			State: session.StateActive,
			Path:  t.TempDir(),
		}

		count, err := svc.Prune(context.Background(), false)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		sessions, _ := store.List(context.Background())
		assert.Len(t, sessions, 1)
		assert.Equal(t, "active", sessions[0].ID)
	})

	t.Run("per-rule max_recycled", func(t *testing.T) {
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules: []config.Rule{
				{Pattern: "", MaxRecycled: intPtr(5)},                     // catch-all default
				{Pattern: "github.com/strict/.*", MaxRecycled: intPtr(1)}, // strict override
			},
		}
		svc := newTestService(t, store, cfg)

		remoteStrict := "https://github.com/strict/repo"
		remoteNormal := "https://github.com/normal/repo"
		baseTime := time.Now()

		// Add 3 sessions for strict repo
		for i := 0; i < 3; i++ {
			sess := session.Session{
				ID:        string(rune('a' + i)),
				Remote:    remoteStrict,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: baseTime.Add(time.Duration(i) * time.Hour),
			}
			store.sessions[sess.ID] = sess
		}

		// Add 3 sessions for normal repo
		for i := 0; i < 3; i++ {
			sess := session.Session{
				ID:        string(rune('x' + i)),
				Remote:    remoteNormal,
				State:     session.StateRecycled,
				Path:      t.TempDir(),
				UpdatedAt: baseTime.Add(time.Duration(i) * time.Hour),
			}
			store.sessions[sess.ID] = sess
		}

		count, err := svc.Prune(context.Background(), false)
		require.NoError(t, err)
		// strict: keep 1, delete 2
		// normal: keep 3 (under limit of 5)
		assert.Equal(t, 2, count)

		sessions, _ := store.List(context.Background())
		assert.Len(t, sessions, 4)

		// Count per remote
		strictCount := 0
		normalCount := 0
		for _, s := range sessions {
			if s.Remote == remoteStrict {
				strictCount++
			} else {
				normalCount++
			}
		}
		assert.Equal(t, 1, strictCount, "strict repo should have 1 session")
		assert.Equal(t, 3, normalCount, "normal repo should have 3 sessions")
	})
}

func TestSessionService_Events(t *testing.T) {
	t.Run("create emits session.created", func(t *testing.T) {
		store := newMockStore()
		tb := testbus.New(t)
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
		}
		svc := newTestServiceWithBus(t, store, cfg, tb.EventBus)

		sess, err := svc.CreateSession(context.Background(), CreateOptions{
			Name:   "test-session",
			Remote: "https://github.com/example/repo.git",
		})
		require.NoError(t, err)

		p := testbus.FindPayload[eventbus.SessionCreatedPayload](tb, t, eventbus.EventSessionCreated)
		assert.Equal(t, sess.ID, p.Session.ID)
		assert.Equal(t, "test-session", p.Session.Name)
	})

	t.Run("recycle emits session.recycled", func(t *testing.T) {
		store := newMockStore()
		tb := testbus.New(t)
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
		}
		svc := newTestServiceWithBus(t, store, cfg, tb.EventBus)

		sessDir := filepath.Join(cfg.ReposDir(), "repo-active-abc")
		require.NoError(t, os.MkdirAll(sessDir, 0o755))

		sess := session.Session{
			ID:     "recycle1",
			Name:   "to-recycle",
			Slug:   "to-recycle",
			State:  session.StateActive,
			Path:   sessDir,
			Remote: "https://github.com/example/repo.git",
		}
		require.NoError(t, store.Save(context.Background(), sess))

		err := svc.RecycleSession(context.Background(), "recycle1", io.Discard)
		require.NoError(t, err)

		p := testbus.FindPayload[eventbus.SessionRecycledPayload](tb, t, eventbus.EventSessionRecycled)
		assert.Equal(t, "recycle1", p.Session.ID)
		assert.Equal(t, session.StateRecycled, p.Session.State)
		assert.Equal(t, sessDir, p.Session.Path, "recycled session path must not change")
	})

	t.Run("rename emits session.renamed", func(t *testing.T) {
		store := newMockStore()
		tb := testbus.New(t)
		svc := newTestServiceWithBus(t, store, nil, tb.EventBus)

		sess := session.Session{
			ID:    "test1",
			Name:  "old-name",
			Slug:  "old-name",
			State: session.StateActive,
		}
		require.NoError(t, store.Save(context.Background(), sess))

		err := svc.RenameSession(context.Background(), "test1", "new-name")
		require.NoError(t, err)

		p := testbus.FindPayload[eventbus.SessionRenamedPayload](tb, t, eventbus.EventSessionRenamed)
		assert.Equal(t, "old-name", p.OldName)
		assert.Equal(t, "new-name", p.Session.Name)
	})

	t.Run("delete emits session.deleted", func(t *testing.T) {
		store := newMockStore()
		tb := testbus.New(t)
		svc := newTestServiceWithBus(t, store, nil, tb.EventBus)

		sess := session.Session{
			ID:    "del1",
			Name:  "to-delete",
			State: session.StateRecycled,
			Path:  t.TempDir(),
		}
		require.NoError(t, store.Save(context.Background(), sess))

		err := svc.DeleteSession(context.Background(), "del1")
		require.NoError(t, err)

		p := testbus.FindPayload[eventbus.SessionDeletedPayload](tb, t, eventbus.EventSessionDeleted)
		assert.Equal(t, "del1", p.SessionID)
	})
}

func TestCreateSession_PathFormat(t *testing.T) {
	store := newMockStore()
	cfg := &config.Config{
		DataDir: t.TempDir(),
		GitPath: "git",
	}
	svc := newTestService(t, store, cfg)

	sess, err := svc.CreateSession(context.Background(), CreateOptions{
		Name:   "my-feature",
		Remote: "https://github.com/example/myrepo.git",
	})
	require.NoError(t, err)

	base := filepath.Base(sess.Path)

	assert.Regexp(t, `^myrepo-[a-z0-9]{6}$`, base,
		"directory name must be {repoName}-{6charID}")
	assert.NotContains(t, base, "my-feature",
		"directory name must not contain the session slug")
	assert.NotEqual(t, fmt.Sprintf("myrepo-%s", sess.ID), base,
		"directory name must not use Session.ID")
}

func TestRecycleSession_PathUnchanged(t *testing.T) {
	store := newMockStore()
	cfg := &config.Config{
		DataDir: t.TempDir(),
		GitPath: "git",
	}
	svc := newTestService(t, store, cfg)

	sessDir := filepath.Join(cfg.ReposDir(), "repo-x7k2qp")
	require.NoError(t, os.MkdirAll(sessDir, 0o755))

	sess := session.Session{
		ID:     "abc123",
		Name:   "my-session",
		Slug:   "my-session",
		State:  session.StateActive,
		Path:   sessDir,
		Remote: "https://github.com/example/repo.git",
	}
	require.NoError(t, store.Save(context.Background(), sess))

	err := svc.RecycleSession(context.Background(), "abc123", io.Discard)
	require.NoError(t, err)

	recycled, err := store.Get(context.Background(), "abc123")
	require.NoError(t, err)
	assert.Equal(t, sessDir, recycled.Path, "directory must not be renamed on recycle")
	assert.Equal(t, session.StateRecycled, recycled.State)
}

func TestCreateSession_RecycledSessionKeepsPath(t *testing.T) {
	store := newMockStore()
	cfg := &config.Config{
		DataDir: t.TempDir(),
		GitPath: "git",
	}
	svc := newTestService(t, store, cfg)

	recycledPath := filepath.Join(cfg.ReposDir(), "repo-x7k2qp")

	recycled := session.Session{
		ID:     "abc123",
		Name:   "old-name",
		Slug:   "old-name",
		State:  session.StateRecycled,
		Path:   recycledPath,
		Remote: "https://github.com/example/repo.git",
	}
	require.NoError(t, store.Save(context.Background(), recycled))

	sess, err := svc.CreateSession(context.Background(), CreateOptions{
		Name:   "new-name",
		Remote: "https://github.com/example/repo.git",
	})
	require.NoError(t, err)

	assert.Equal(t, "abc123", sess.ID, "should reuse the recycled session record")
	assert.Equal(t, recycledPath, sess.Path, "path must not change on reactivation")
	assert.Equal(t, "new-name", sess.Name)
	assert.Equal(t, session.StateActive, sess.State)
}

func TestCreateSessionWithWindows_RollbackOnRunShFailure(t *testing.T) {
	store := newMockStore()
	cfg := &config.Config{
		DataDir: t.TempDir(),
		GitPath: "git",
	}
	svc := newTestService(t, store, cfg)

	req := action.NewSessionRequest{
		Name:   "test-session",
		Remote: "https://github.com/test/repo",
		ShCmd:  "exit 1", // fails: directory won't exist since mockGit.Clone is a no-op
	}

	err := svc.CreateSessionWithWindows(context.Background(), req, nil, false)
	require.Error(t, err)

	// Session must be cleaned up from the store after RunSh failure.
	sessions, listErr := store.List(context.Background())
	require.NoError(t, listErr)
	assert.Empty(t, sessions, "session should not remain in store after RunSh failure")
}

func TestSetSessionGroup(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	past := time.Now().Add(-1 * time.Hour)
	sess := session.Session{
		ID:        "test1",
		Name:      "my-session",
		State:     session.StateActive,
		CreatedAt: past,
		UpdatedAt: past,
	}
	require.NoError(t, store.Save(context.Background(), sess))

	err := svc.SetSessionGroup(context.Background(), "test1", "backend")
	require.NoError(t, err)

	updated, err := store.Get(context.Background(), "test1")
	require.NoError(t, err)
	assert.Equal(t, "backend", updated.Group())
	assert.True(t, updated.UpdatedAt.After(past))
}

func TestSetSessionGroup_NotFound(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	err := svc.SetSessionGroup(context.Background(), "nonexistent", "backend")
	assert.Error(t, err)
}

func TestSetSessionGroup_ClearGroup(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	sess := session.Session{
		ID:       "test1",
		Name:     "my-session",
		State:    session.StateActive,
		Metadata: map[string]string{"group": "backend"},
	}
	require.NoError(t, store.Save(context.Background(), sess))

	err := svc.SetSessionGroup(context.Background(), "test1", "")
	require.NoError(t, err)

	updated, err := store.Get(context.Background(), "test1")
	require.NoError(t, err)
	assert.Empty(t, updated.Group())
}

func TestSetSessionGroup_TrimWhitespace(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, nil)

	sess := session.Session{
		ID:    "test1",
		Name:  "my-session",
		State: session.StateActive,
	}
	require.NoError(t, store.Save(context.Background(), sess))

	err := svc.SetSessionGroup(context.Background(), "test1", "  backend  ")
	require.NoError(t, err)

	updated, err := store.Get(context.Background(), "test1")
	require.NoError(t, err)
	assert.Equal(t, "backend", updated.Group())
}

// Ensure the mock implements the interface at compile time.
var (
	_ git.Git       = (*mockGit)(nil)
	_ session.Store = (*mockStore)(nil)
)
