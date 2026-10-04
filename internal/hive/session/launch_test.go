package session

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
)

// newRealGitService runs git for real, because the failure draft is about what
// git leaves behind and says, which a stub would only repeat back.
func newRealGitService(t *testing.T, cfg *config.Config) *Service {
	t.Helper()
	exec := &executil.RealExecutor{}
	return NewService(zerolog.Nop(), newMockStore(), git.NewExecutor("git", exec), cfg, testbus.New(t).EventBus, exec,
		tmpl.New(tmpl.Config{}), PlainStyler{}, io.Discard, io.Discard, testMultiplexer{})
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// failingPostCheckoutRepo is a clonable remote plus a global post-checkout
// hook that exits non-zero. git propagates the hook's exit code, so the
// checkout is complete and the clone still failed.
func failingPostCheckoutRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(src, 0o755))
	runGit(t, src, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("hi"), 0o644))
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=a@b.c", "-c", "user.name=a", "commit", "-qm", "init")

	hooks := filepath.Join(root, "hooks")
	require.NoError(t, os.MkdirAll(hooks, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	// GIT_CONFIG_* reaches the clone the way a global hook would, without
	// touching the machine.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	return src
}

func TestCreateFromRequestReportsTheFailedOperationAndItsCheckout(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	remote := failingPostCheckoutRepo(t)

	_, err := newRealGitService(t, cfg).CreateFromRequest(t.Context(), LaunchRequest{Name: "review-81", Repo: remote})

	var failure *LaunchError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "review-81", failure.Name)
	assert.Equal(t, remote, failure.Remote)

	// Off CreateSessionError, not derived from the progress lines.
	assert.Equal(t, "clone repository", failure.Step)
	assert.Equal(t, "full", failure.CloneStrategy)
	assert.Equal(t, cfg.ReposDir(), filepath.Dir(failure.Destination))
	assert.True(t, failure.LeftoverCheckout, "a hook that fails after checkout leaves it complete")
	assert.DirExists(t, failure.Destination, "the complete checkout the failed clone left behind")

	// The error names one operation; the tail names the sequence that led to it.
	assert.Contains(t, failure.Output, "Clone strategy: full")
	assert.Contains(t, failure.Output, "Cloning repository...")
	assert.Contains(t, err.Error(), "clone repository", "the step travels in the surfaced error")
}

func TestCreateFromRequestReportsWhyACloneWasRefused(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	missing := filepath.Join(t.TempDir(), "no-such-repo")

	_, err := newRealGitService(t, cfg).CreateFromRequest(t.Context(), LaunchRequest{Name: "review-81", Repo: missing})

	var failure *LaunchError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "clone repository", failure.Step)
	assert.Equal(t, "full", failure.CloneStrategy)
	assert.Contains(t, err.Error(), "does not exist", "git's words, not just its exit status")

	// A destination is named either way; git removed this one, so there is
	// nothing to tell the user to delete.
	assert.NotEmpty(t, failure.Destination)
	assert.False(t, failure.LeftoverCheckout)
	assert.NoDirExists(t, failure.Destination)
}

func TestCreateFromRequestPrefersAnEquivalentConfiguredCheckout(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspaces")
	checkout := filepath.Join(workspaceDir, "hive")
	require.NoError(t, mkdirGitDir(checkout))

	cfg := &config.Config{DataDir: t.TempDir(), Workspaces: []string{workspaceDir}}
	svc := newTestService(t, newMockStore(), cfg)
	svc.git = &launchOptionsGit{remote: "git@github.com:colonyops/hive.git"}

	sess, err := svc.CreateFromRequest(t.Context(), LaunchRequest{
		Name: "review-pr-1", Prompt: "Review this", Repo: "https://github.com/colonyops/hive.git",
		Tags: []string{"acme/repo#1", "", "acme/repo#2", "acme/repo#1"},
	})
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:colonyops/hive.git", sess.Remote, "the configured checkout's spelling wins")
	assert.Equal(t, "review-pr-1", sess.Name)
	assert.Equal(t, []string{"acme/repo#1", "acme/repo#2"}, sess.Tags)
}

// A taken name is the caller's to resolve with another name, so it arrives
// as the sentinel rather than as a failure draft.
func TestCreateFromRequestPassesADuplicateNameThrough(t *testing.T) {
	store := newMockStore()
	store.sessions["taken"] = session.Session{ID: "taken", Name: "review", Slug: "review", State: session.StateActive}
	svc := newTestService(t, store, nil)

	_, err := svc.CreateFromRequest(t.Context(), LaunchRequest{Name: "review", Repo: testRemote})
	require.ErrorIs(t, err, session.ErrDuplicateName)
	var failure *LaunchError
	assert.NotErrorAs(t, err, &failure)
}

func TestProgressLogKeepsTheLastLineAndBoundsTheTail(t *testing.T) {
	progress := &progressLog{}
	for i := range maxProgressTailLines + 5 {
		_, _ = progress.Write([]byte("step " + string(rune('a'+i%26)) + "\n"))
	}
	_, _ = progress.Write([]byte("Cloning repository...\n"))

	assert.Equal(t, "Cloning repository...", progress.LastLine())
	tail := progress.Tail()
	assert.LessOrEqual(t, len(tail), maxProgressTail)
	assert.Contains(t, tail, "Cloning repository...")
	assert.NotContains(t, tail, "step a", "the oldest lines fall out of a bounded tail")
}

func TestProgressLogSplitsPartialWritesIntoLines(t *testing.T) {
	progress := &progressLog{}
	_, _ = progress.Write([]byte("Executing rules...\nhook: "))
	_, _ = progress.Write([]byte("command not found\n\n"))

	assert.Equal(t, "hook: command not found", progress.LastLine())
	assert.Equal(t, "Executing rules...\nhook: command not found", progress.Tail())
}
