package hive

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingLookPath resolves only the names in found and records every lookup.
type recordingLookPath struct {
	found  map[string]bool
	lookup []string
}

func (r *recordingLookPath) LookPath(_ context.Context, name string) (string, error) {
	r.lookup = append(r.lookup, name)
	if r.found[name] {
		return "/usr/local/bin/" + name, nil
	}
	return "", errors.New("executable file not found in $PATH")
}

type preflightFixture struct {
	svc    *SessionService
	store  *mockStore
	clones int
	lp     *recordingLookPath
}

func newPreflightFixture(t *testing.T, cfg *config.Config, renderer *tmpl.Renderer, found ...string) *preflightFixture {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	cfg.GitPath = "git"
	f := &preflightFixture{store: newMockStore(), lp: &recordingLookPath{found: map[string]bool{}}}
	for _, name := range found {
		f.lp.found[name] = true
	}
	gitImpl := &capturingMockGit{CloneFn: func(context.Context, string, string) error {
		f.clones++
		return nil
	}}
	f.svc = NewSessionService(f.store, gitImpl, cfg, testbus.New(t).EventBus, &tmuxExec{}, renderer, zerolog.New(io.Discard), io.Discard, io.Discard)
	f.svc.SetLookPath(f.lp.LookPath)
	return f
}

func TestCreateSession_MissingDefaultAgentFailsBeforeClone(t *testing.T) {
	cfg := &config.Config{Agents: config.AgentsConfig{Default: "pi"}}
	f := newPreflightFixture(t, cfg, tmpl.New(tmpl.Config{AgentCommand: "pi", AgentWindow: "pi"}))

	_, err := f.svc.CreateSession(context.Background(), CreateOptions{Name: "review", Remote: testRemote, Background: true})

	var notFound *AgentNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, "pi", notFound.Key)
	assert.Equal(t, "pi", notFound.Command)
	require.EqualError(t, err, `agent "pi": command "pi" not found on PATH`)
	assert.Zero(t, f.clones, "must fail before cloning")
	assert.Empty(t, f.store.sessions)
}

func TestCreateSession_AgentPreflightUsesResolvedProfile(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Default: "claude",
			Profiles: map[string]config.AgentProfile{
				"claude": {Command: "claude"},
				"aider":  {Command: "aider-bin"},
			},
		},
		Rules: []config.Rule{{Pattern: "", Agent: "aider"}},
	}

	t.Run("rule agent", func(t *testing.T) {
		f := newPreflightFixture(t, cfg, tmpl.New(tmpl.Config{AgentCommand: "claude", AgentWindow: "claude"}), "claude")
		_, err := f.svc.CreateSession(context.Background(), CreateOptions{Name: "rule", Remote: testRemote, Background: true})

		var notFound *AgentNotFoundError
		require.ErrorAs(t, err, &notFound)
		assert.Equal(t, "aider", notFound.Key)
		assert.Equal(t, []string{"aider-bin"}, f.lp.lookup)
	})

	t.Run("agent key overrides rule agent", func(t *testing.T) {
		f := newPreflightFixture(t, cfg, tmpl.New(tmpl.Config{AgentCommand: "claude", AgentWindow: "claude"}), "claude")
		_, err := f.svc.CreateSession(context.Background(), CreateOptions{Name: "key", Remote: testRemote, Background: true, AgentKey: "claude"})

		require.NoError(t, err)
		assert.Equal(t, []string{"claude"}, f.lp.lookup)
	})
}

func TestCreateSession_AgentPreflightSkipped(t *testing.T) {
	renderer := tmpl.New(tmpl.Config{AgentCommand: "pi", AgentWindow: "pi"})

	t.Run("windows without agentCommand", func(t *testing.T) {
		cfg := &config.Config{Rules: []config.Rule{{Pattern: "", Windows: []config.WindowConfig{{Name: "editor", Command: "nvim ."}}}}}
		f := newPreflightFixture(t, cfg, renderer)

		_, err := f.svc.CreateSession(context.Background(), CreateOptions{Name: "editor", Remote: testRemote, Background: true})
		require.NoError(t, err)
		assert.Empty(t, f.lp.lookup)
	})

	t.Run("skip spawn", func(t *testing.T) {
		f := newPreflightFixture(t, &config.Config{}, renderer)

		_, err := f.svc.CreateSession(context.Background(), CreateOptions{Name: "nospawn", Remote: testRemote, SkipSpawn: true})
		require.NoError(t, err)
		assert.Empty(t, f.lp.lookup)
	})
}

func TestCreateSession_AgentCommandPath(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "agent")
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755))
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("text"), 0o644))

	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{name: "executable file", command: executable + " --flag"},
		{name: "missing file", command: filepath.Join(dir, "missing"), wantErr: true},
		{name: "not executable", command: plain, wantErr: true},
		{name: "directory", command: dir, wantErr: true},
		{name: "relative path is not checked", command: "./bin/agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPreflightFixture(t, &config.Config{}, tmpl.New(tmpl.Config{AgentCommand: tt.command, AgentWindow: "custom"}))

			_, err := f.svc.CreateSession(context.Background(), CreateOptions{Name: "path", Remote: testRemote, Background: true})
			assert.Empty(t, f.lp.lookup, "paths are checked directly, not searched")
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			var notFound *AgentNotFoundError
			require.ErrorAs(t, err, &notFound)
			assert.Contains(t, err.Error(), "is not executable")
		})
	}
}

func TestCheckAgent(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Default: "pi",
			Profiles: map[string]config.AgentProfile{
				"pi":     {},
				"claude": {Command: "claude"},
			},
		},
	}
	f := newPreflightFixture(t, cfg, tmpl.New(tmpl.Config{AgentCommand: "pi", AgentWindow: "pi"}), "claude")

	require.NoError(t, f.svc.CheckAgent(context.Background(), "claude"))

	var notFound *AgentNotFoundError
	require.ErrorAs(t, f.svc.CheckAgent(context.Background(), ""), &notFound)
	assert.Equal(t, "pi", notFound.Key)

	require.ErrorContains(t, f.svc.CheckAgent(context.Background(), "missing"), `unknown agent "missing"`)
}

func TestAgentExecutable(t *testing.T) {
	tests := map[string]string{
		"claude":                  "claude",
		"  claude --model sonnet": "claude",
		"FOO=1 BAR_2=x pi --yolo": "pi",
		"/opt/bin/agent":          "/opt/bin/agent",
		"$HOME/bin/agent":         "",
		"'quoted agent'":          "",
		"":                        "",
		"=notassignment agent":    "=notassignment",
		"1X=bad agent":            "1X=bad",
	}
	for command, want := range tests {
		assert.Equal(t, want, agentExecutable(command), command)
	}
}

// tmuxExec fakes tmux for spawn tests: has-session reports hasSession, and
// the spawn command run through sh returns spawnErr.
type tmuxExec struct {
	mu         sync.Mutex
	hasSession bool
	spawnErr   error
	calls      [][]string
}

func (e *tmuxExec) Run(_ context.Context, cmd string, args ...string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, append([]string{cmd}, args...))
	if cmd == "tmux" && len(args) > 0 && args[0] == "has-session" && !e.hasSession {
		return nil, errors.New("can't find session")
	}
	return nil, nil
}

func (e *tmuxExec) RunDir(ctx context.Context, _, cmd string, args ...string) ([]byte, error) {
	return e.Run(ctx, cmd, args...)
}

func (e *tmuxExec) RunStream(ctx context.Context, _, _ io.Writer, cmd string, args ...string) error {
	if _, err := e.Run(ctx, cmd, args...); err != nil {
		return err
	}
	if cmd == "sh" {
		return e.spawnErr
	}
	return nil
}

func (e *tmuxExec) RunDirStream(ctx context.Context, _ string, stdout, stderr io.Writer, cmd string, args ...string) error {
	return e.RunStream(ctx, stdout, stderr, cmd, args...)
}

func (e *tmuxExec) ran(args ...string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.calls {
		if assert.ObjectsAreEqual(args, c) {
			return true
		}
	}
	return false
}

func TestCreateSession_SpawnFailure(t *testing.T) {
	spawnErr := errors.New("spawn exploded")
	newService := func(t *testing.T, exec *tmuxExec) (*SessionService, *mockStore) {
		t.Helper()
		store := newMockStore()
		cfg := &config.Config{
			DataDir: t.TempDir(),
			GitPath: "git",
			Rules:   []config.Rule{{Pattern: "", Spawn: []string{"launch {{ .Name }}"}}},
		}
		svc := NewSessionService(store, &mockGit{}, cfg, testbus.New(t).EventBus, exec, tmpl.New(tmpl.Config{}), zerolog.New(io.Discard), io.Discard, io.Discard)
		return withAgentsOnPath(svc), store
	}

	t.Run("deletes the session", func(t *testing.T) {
		exec := &tmuxExec{spawnErr: spawnErr}
		svc, store := newService(t, exec)

		_, err := svc.CreateSession(context.Background(), CreateOptions{Name: "Broken Spawn", Remote: testRemote})
		require.ErrorIs(t, err, spawnErr)

		var createErr *CreateSessionError
		require.ErrorAs(t, err, &createErr)
		assert.Equal(t, "spawn terminal", createErr.Operation)
		assert.NotEmpty(t, createErr.Destination)
		assert.Equal(t, config.CloneStrategyFull, createErr.CloneStrategy)

		assert.Empty(t, store.sessions, "no active session without a terminal may remain")
		assert.True(t, exec.ran("tmux", "kill-session", "-t", "=broken-spawn"), "tmux kill must match the slug exactly")
	})

	t.Run("keeps the session when its tmux session exists", func(t *testing.T) {
		exec := &tmuxExec{spawnErr: spawnErr, hasSession: true}
		svc, store := newService(t, exec)

		_, err := svc.CreateSession(context.Background(), CreateOptions{Name: "Attach Failed", Remote: testRemote})
		require.ErrorIs(t, err, spawnErr)
		assert.Len(t, store.sessions, 1)
		assert.True(t, exec.ran("tmux", "has-session", "-t", "=attach-failed"))
	})
}
