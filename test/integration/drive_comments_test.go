//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriveCommentsFollowPushesAndAnswerFromTheCLI(t *testing.T) {
	t.Parallel()
	server := NewHarness(t).StartDriveServer(t)
	h := NewHarness(t).UseDriveServer(server)
	workDir := driveWorkDir(t, h, "drive-comments-repo")
	target, err := os.Readlink(filepath.Join(workDir, ".hive"))
	require.NoError(t, err)
	owner, repo := filepath.Base(filepath.Dir(target)), filepath.Base(target)
	plan := filepath.Join(target, "plans", "ship.md")

	push := func(body string) string {
		t.Helper()
		require.NoError(t, os.WriteFile(plan, []byte(body), 0o644))
		lines, err := h.RunJSONLinesInDir(workDir, "drive", "push", "--json", "plans/ship.md")
		require.NoError(t, err)
		require.Len(t, lines, 1)
		sha, _ := lines[0]["sha256"].(string)
		return sha
	}
	threads := func(args ...string) []map[string]any {
		t.Helper()
		lines, err := h.RunJSONLinesInDir(workDir, append([]string{"drive", "comments", "--json"}, append(args, "plans/ship.md")...)...)
		require.NoError(t, err)
		return lines
	}
	anchor := func(th map[string]any) map[string]any {
		a, _ := th["anchor"].(map[string]any)
		return a
	}

	sha := push("# Ship\n\nKeep the old ones for now.\n")

	body, err := json.Marshal(map[string]any{
		"document_sha256": sha,
		"anchor":          map[string]any{"start_line": 3, "end_line": 3, "quote": "Keep the old ones"},
		"body":            "Why keep them?",
	})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		server.URL+"/api/v1/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/threads/plans/ship.md", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+server.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	t.Run("a push above the quote moves the thread", func(t *testing.T) {
		push("# Ship\n\nIntro.\n\nKeep the old ones for now.\n")
		got := threads()
		require.Len(t, got, 1)
		assert.InDelta(t, 5, anchor(got[0])["start_line"], 0)
		assert.Equal(t, false, got[0]["outdated"])
	})

	t.Run("a push without the quote outdates it", func(t *testing.T) {
		push("# Ship\n\nIntro.\n")
		got := threads()
		require.Len(t, got, 1)
		assert.Equal(t, true, got[0]["outdated"])
		assert.Equal(t, "Keep the old ones", anchor(got[0])["quote"])
	})

	t.Run("reply and resolve from the CLI", func(t *testing.T) {
		id, _ := threads()[0]["id"].(string)
		require.NotEmpty(t, id)
		suffix := id[len(id)-8:]

		out, err := h.RunInDir(workDir, "drive", "comments", "reply", "plans/ship.md", suffix, "Removed them.")
		require.NoError(t, err, out)
		out, err = h.RunInDir(workDir, "drive", "comments", "resolve", "plans/ship.md", suffix)
		require.NoError(t, err, out)

		assert.Empty(t, threads(), "a resolved thread leaves the default list")
		all := threads("--all")
		require.Len(t, all, 1)
		assert.NotEmpty(t, all[0]["resolved_at"])
		comments, _ := all[0]["comments"].([]any)
		require.Len(t, comments, 2)
		reply, _ := comments[1].(map[string]any)
		assert.Equal(t, "Removed them.", reply["body"])
		author, _ := reply["author"].(map[string]any)
		assert.NotEmpty(t, author["agent"], "a token-written reply names its token")
	})

	t.Run("the text list shows the thread", func(t *testing.T) {
		out, err := h.RunStdoutInDir(workDir, "drive", "comments", "--all", "plans/ship.md")
		require.NoError(t, err)
		assert.Contains(t, out, "0 open, 0 outdated, 1 resolved")
		assert.Contains(t, out, `"Keep the old ones"`)
	})
}
