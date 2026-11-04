package session

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/pkg/executil/executiltest"
	"github.com/colonyops/hive/pkg/tmpl"
)

const chatUUID = "6f1c1f0e-8f0a-4f3e-9a55-2a1d6c8b9e10"

func TestResolveCaller(t *testing.T) {
	t.Parallel()
	checkout := t.TempDir()
	other := t.TempDir()
	// A path that does not exist is not symlink-resolved, so the test uses real ones.
	sub := filepath.Join(checkout, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	store := newMockStore()
	store.sessions["abc123"] = session.Session{ID: "abc123", Name: "one", Path: checkout, State: session.StateActive}
	store.sessions["def456"] = session.Session{ID: "def456", Name: "two", Path: other, State: session.StateActive}
	store.sessions["204815"] = session.Session{ID: "204815", Name: "digits", Path: t.TempDir(), State: session.StateActive}
	svc := newTestService(t, store, nil)
	ctx := context.Background()

	tests := []struct {
		name                string
		explicit, env, cwd  string
		wantKind            CallerKind
		wantSession, wantID string
		wantErr             ops.Kind
	}{
		{name: "global", explicit: "global", wantKind: CallerKindGlobal},
		{name: "uuid is a chat", explicit: chatUUID, wantKind: CallerKindChat, wantID: chatUUID},
		{name: "positive integer is a chat", env: "42", wantKind: CallerKindChat, wantID: "42"},
		{name: "an all-digit session id is the session", env: "204815", wantKind: CallerKindSession, wantSession: "204815"},
		{name: "zero is not a chat", explicit: "0", wantErr: ops.KindNotFound},
		{name: "absolute path inside a session", explicit: sub, wantKind: CallerKindSession, wantSession: "abc123"},
		{name: "absolute path inside no session", explicit: filepath.Join(t.TempDir(), "x"), wantErr: ops.KindNotFound},
		{name: "session id", explicit: "abc123", wantKind: CallerKindSession, wantSession: "abc123"},
		{name: "local session id", explicit: "local:abc123", wantKind: CallerKindSession, wantSession: "abc123"},
		{name: "missing session id", explicit: "zzz999", wantErr: ops.KindNotFound},
		{name: "another server", explicit: "server-b:abc123", wantErr: ops.KindInvalid},
		{name: "relative path", explicit: "./here", wantErr: ops.KindInvalid},
		{name: "empty id after local", explicit: "local:", wantErr: ops.KindInvalid},
		{name: "env decides before cwd", env: "def456", cwd: checkout, wantKind: CallerKindSession, wantSession: "def456"},
		{name: "explicit decides before env", explicit: "abc123", env: "def456", wantKind: CallerKindSession, wantSession: "abc123"},
		{name: "a bad explicit does not fall back", explicit: "zzz999", env: "def456", cwd: checkout, wantErr: ops.KindNotFound},
		{name: "a bad env does not fall back to cwd", env: "zzz999", cwd: checkout, wantErr: ops.KindNotFound},
		{name: "cwd inside a session", cwd: sub, wantKind: CallerKindSession, wantSession: "abc123"},
		{name: "cwd inside no session is not global", cwd: t.TempDir(), wantErr: ops.KindNotFound},
		{name: "nothing given", wantErr: ops.KindInvalid},
		{name: "blank values count as empty", explicit: "  ", env: "", cwd: checkout, wantKind: CallerKindSession, wantSession: "abc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.ResolveCaller(ctx, tt.explicit, tt.env, tt.cwd)
			if tt.wantErr != "" {
				var opErr *ops.Error
				require.ErrorAs(t, err, &opErr)
				assert.Equal(t, tt.wantErr, opErr.Kind, opErr.Msg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, got.Kind)
			assert.Equal(t, tt.wantSession, got.Session.ID)
			assert.Equal(t, tt.wantID, got.ChatID)
		})
	}
}

type recordingMultiplexer struct {
	testMultiplexer
	created []multiplexer.SessionSpec
	opened  []multiplexer.SessionSpec
}

func (m *recordingMultiplexer) CreateSession(_ context.Context, spec multiplexer.SessionSpec) error {
	m.created = append(m.created, spec)
	return nil
}

func (m *recordingMultiplexer) OpenSession(_ context.Context, spec multiplexer.SessionSpec, _ multiplexer.Target) (multiplexer.LaunchResult, error) {
	m.opened = append(m.opened, spec)
	return multiplexer.LaunchResult{Created: true}, nil
}

func newRecordingService(t *testing.T, store session.Store) (*Service, *recordingMultiplexer) {
	t.Helper()
	mux := &recordingMultiplexer{}
	cfg := &config.Config{
		DataDir: t.TempDir(),
		GitPath: "git",
		Rules:   []config.Rule{{Windows: []config.WindowConfig{{Name: "agent"}}}},
	}
	svc := NewService(zerolog.New(io.Discard), store, &mockGit{}, cfg, testbus.New(t).EventBus,
		&executiltest.Exec{}, tmpl.New(tmpl.Config{}), PlainStyler{}, io.Discard, io.Discard, mux)
	return svc, mux
}

func TestEveryTmuxCreatePathSetsTheCallerEnv(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		t.Parallel()
		svc, mux := newRecordingService(t, newMockStore())
		sess, err := svc.CreateSession(ctx, CreateOptions{Name: "one", Remote: testRemote, Background: true, Env: []string{"A=b"}})
		require.NoError(t, err)
		require.Len(t, mux.created, 1)
		assert.Equal(t, []string{"A=b", "HIVE_AGENT_SESSION=" + sess.ID}, mux.created[0].Env)
	})

	t.Run("create with windows", func(t *testing.T) {
		t.Parallel()
		svc, mux := newRecordingService(t, newMockStore())
		sess, err := svc.CreateSessionWithWindows(ctx, NewSessionRequest{Name: "two", Remote: testRemote},
			[]multiplexer.WindowSpec{{Name: "w"}}, true)
		require.NoError(t, err)
		require.Len(t, mux.created, 1)
		assert.Equal(t, []string{"HIVE_AGENT_SESSION=" + sess.ID}, mux.created[0].Env)
	})

	t.Run("reopen", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		dir := filepath.Join(t.TempDir(), "checkout")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		store.sessions["abc123"] = session.Session{ID: "abc123", Name: "one", Slug: "one", Path: dir, State: session.StateActive}
		svc, mux := newRecordingService(t, store)
		_, err := svc.OpenTmuxSession(ctx, "one", dir, testRemote, "", true)
		require.NoError(t, err)
		require.Len(t, mux.opened, 1)
		assert.Equal(t, []string{"HIVE_AGENT_SESSION=abc123"}, mux.opened[0].Env)
	})
}
