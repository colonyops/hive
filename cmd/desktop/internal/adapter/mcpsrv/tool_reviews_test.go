package mcpsrv_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

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

// seedBranchSession makes a hive session whose checkout is on a branch with
// one changed file, a.go, against main.
func seedBranchSession(t *testing.T, core *app.App, id string) string {
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nvar x = 2\nvar y = 3\n"), 0o600))
	gitIn(t, dir, "commit", "-qam", "work")

	_, err := core.HiveConn().ExecContext(t.Context(),
		`INSERT INTO sessions (id, name, slug, path, remote, state, clone_strategy, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'https://github.com/acme/repo', 'active', 'full', 1, 1)`,
		id, "feature-"+id, "feature-"+id, dir)
	require.NoError(t, err)
	return dir
}

type diffReviewAnswer struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	Agent    string `json:"agent"`
	Head     string `json:"head"`
	Summary  string `json:"summary"`
	Comments []struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Path      string `json:"path"`
		StartLine int    `json:"startLine"`
		EndLine   int    `json:"endLine"`
		Quote     string `json:"quote"`
		Severity  string `json:"severity"`
	} `json:"comments"`
}

func TestDiffReviewFromTheSessionsOwnAgent(t *testing.T) {
	t.Parallel()
	core, session := testCanvasSession(t)
	dir := seedBranchSession(t, core, "s1")

	var put diffReviewAnswer
	call(t, session, "diff_review_put", map[string]any{
		"caller": dir, "agent": "claude", "summary": "One bug.",
		"comments": []map[string]any{
			{"path": "a.go", "startLine": 3, "endLine": 4, "text": "x changed", "severity": "issue"},
			{"path": "a.go", "kind": "file", "text": "add a test"},
		},
	}, &put)
	assert.Equal(t, "claude", put.Author, "a session's agent is the author when none is named")
	assert.Len(t, put.Head, 40)
	require.Len(t, put.Comments, 2)
	assert.Equal(t, "+var x = 2\n+var y = 3", put.Comments[0].Quote)
	assert.Equal(t, "lines", put.Comments[0].Kind)
	assert.Equal(t, "file", put.Comments[1].Kind)
	assert.Equal(t, "suggestion", put.Comments[1].Severity)

	var got struct {
		Reviews []diffReviewAnswer `json:"reviews"`
	}
	call(t, session, "diff_review_get", map[string]any{"caller": dir}, &got)
	require.Len(t, got.Reviews, 1)
	assert.Equal(t, put.ID, got.Reviews[0].ID)

	text := callErr(t, session, "diff_review_put", map[string]any{
		"caller": dir, "comments": []map[string]any{{"path": "a.go", "startLine": 90, "text": "far"}},
	})
	assert.Contains(t, text, "invalid")
	assert.Contains(t, text, "L90 is not a run of rows")

	var cleared struct {
		Removed int `json:"removed"`
	}
	call(t, session, "diff_review_clear", map[string]any{"caller": dir}, &cleared)
	assert.Equal(t, 1, cleared.Removed)
}

func TestDiffReviewFromAChatNamesTheSession(t *testing.T) {
	t.Parallel()
	core, session := testCanvasSession(t)
	seedBranchSession(t, core, "s1")
	chat := seedAgentSession(t, core, "pr-reviewer", "chat")

	text := callErr(t, session, "diff_review_put", map[string]any{"caller": chat, "summary": "x"})
	assert.Contains(t, text, "name one in sessionId")

	var put diffReviewAnswer
	call(t, session, "diff_review_put", map[string]any{"caller": chat, "sessionId": "feature-s1", "summary": "Fine."}, &put)
	assert.Equal(t, "pr-reviewer", put.Author, "a chat's workspace is the author when none is named")

	text = callErr(t, session, "diff_review_get", map[string]any{"caller": chat, "sessionId": "nope"})
	assert.Contains(t, text, "not_found")
}
