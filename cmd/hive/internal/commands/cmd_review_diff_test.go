package commands

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/reviews"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/pkg/executil"
)

// realGitApp is testApp with a real executor, because a diff review reads
// the session's branch diff through git.
func realGitApp(t *testing.T) (*app.App, *store.SessionStore) {
	t.Helper()
	cfg := &config.Config{Config: *hivetest.LoadConfig(t, t.TempDir())}
	database, err := hive.OpenDB(t.Context(), t.TempDir(), cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	engine, err := hive.New(&cfg.Config, hive.Ports{
		DB: database, Bus: testbus.New(t).EventBus, Executor: &executil.RealExecutor{},
		Mux: hivetest.Mux{}, DataDir: t.TempDir(), Logger: zerolog.Nop(),
	})
	require.NoError(t, err)
	return app.NewApp(zerolog.Nop(), engine, cfg, nil, nil, nil, nil, ""), store.NewSessionStore(database)
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func branchCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nvar x = 1\n"), 0o600))
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-qm", "base")
	gitIn(t, dir, "remote", "add", "origin", dir)
	gitIn(t, dir, "fetch", "-q", "origin")
	gitIn(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	gitIn(t, dir, "checkout", "-qb", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nvar x = 2\n"), 0o600))
	gitIn(t, dir, "commit", "-qam", "work")
	return dir
}

func TestReviewDiffPutShowClear(t *testing.T) {
	a, sessions := realGitApp(t)
	dir := branchCheckout(t)
	now := time.Now()
	require.NoError(t, sessions.Save(t.Context(), session.Session{
		ID: "s1", Name: "feature", Slug: "feature", Path: dir, Remote: flagTestRemote,
		State: session.StateActive, CreatedAt: now, UpdatedAt: now,
	}))
	register := NewReviewCmd(&Flags{}, a).Register

	reviewFile := filepath.Join(t.TempDir(), "review.json")
	require.NoError(t, os.WriteFile(reviewFile, []byte(`{
		"agent": "codex", "summary": "One change.",
		"comments": [{"path": "a.go", "startLine": 3, "severity": "note", "text": "x is 2 now"}]
	}`), 0o600))

	out, err := runRoot(t, register, "review", "diff", "put", "--session-id", "feature", "-f", reviewFile)
	require.NoError(t, err)
	var put struct {
		Author   string `json:"author"`
		Comments []struct {
			Quote string `json:"quote"`
		} `json:"comments"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &put))
	assert.Equal(t, "codex", put.Author, "author defaults to agent")
	require.Len(t, put.Comments, 1)
	assert.Equal(t, "+var x = 2", put.Comments[0].Quote)

	out, err = runRoot(t, register, "review", "diff", "show", "--session-id", "s1")
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(out, "\n"))

	out, err = runRoot(t, register, "review", "diff", "clear", "--session-id", "s1", "--author", "codex")
	require.NoError(t, err)
	assert.JSONEq(t, `{"removed":1}`, out)
}

func TestReviewDiffRefusals(t *testing.T) {
	t.Chdir(t.TempDir())
	a, sessions := realGitApp(t)
	dir := branchCheckout(t)
	now := time.Now()
	require.NoError(t, sessions.Save(t.Context(), session.Session{
		ID: "s1", Name: "feature", Slug: "feature", Path: dir, Remote: flagTestRemote,
		State: session.StateActive, CreatedAt: now, UpdatedAt: now,
	}))
	register := NewReviewCmd(&Flags{}, a).Register

	_, err := runRoot(t, register, "review", "diff", "show")
	require.Error(t, err)
	assert.Equal(t, ops.KindInvalid, ErrorKind(err), "outside a session with no --session-id")

	_, err = runRoot(t, register, "review", "diff", "show", "--session-id", "nope")
	require.Error(t, err)
	assert.Equal(t, ops.KindNotFound, ErrorKind(err))

	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{"agent":"claude","comments":[{"path":"b.go","text":"x"}]}`), 0o600))
	_, err = runRoot(t, register, "review", "diff", "put", "--session-id", "s1", "-f", bad)
	require.Error(t, err)
	assert.Equal(t, ops.KindInvalid, ErrorKind(err))
	assert.Contains(t, err.Error(), "the file is not in the branch diff")
}

func TestReviewDiffPutInputAuthor(t *testing.T) {
	assert.Equal(t, "pr-bot", putInput("s1", reviews.DiffReviewPutIn{Author: " pr-bot ", Agent: "claude"}).Author)
	assert.Equal(t, "claude", putInput("s1", reviews.DiffReviewPutIn{Agent: "claude"}).Author)
	assert.Equal(t, "agent", putInput("s1", reviews.DiffReviewPutIn{}).Author)
}
