//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func byPath(lines []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, line := range lines {
		path, _ := line["path"].(string)
		out[path] = line
	}
	return out
}

func TestDriveRemotePushPullShare(t *testing.T) {
	server := NewHarness(t).StartDriveServer(t)

	pusher := NewHarness(t).UseDriveServer(server)
	workDir := driveWorkDir(t, pusher, "drive-remote-repo")
	target, err := os.Readlink(filepath.Join(workDir, ".hive"))
	require.NoError(t, err)
	repo := filepath.Base(filepath.Dir(target)) + "/" + filepath.Base(target)

	t.Run("push uploads markdown and skips the rest", func(t *testing.T) {
		lines, err := pusher.RunJSONLinesInDir(workDir, "drive", "push", "--json")
		require.NoError(t, err)
		got := byPath(lines)
		require.Len(t, got, 1, "notes.txt is not pushable and is not listed: %v", lines)
		assert.Equal(t, "uploaded", got["plans/ship.md"]["action"])
		assert.Equal(t, sha256Hex("# Ship\n"), got["plans/ship.md"]["sha256"])
		assert.NotContains(t, got["plans/ship.md"], "error")
	})

	t.Run("push again skips unchanged and force uploads", func(t *testing.T) {
		lines, err := pusher.RunJSONLinesInDir(workDir, "drive", "push", "--json")
		require.NoError(t, err)
		assert.Equal(t, "unchanged", byPath(lines)["plans/ship.md"]["action"])

		lines, err = pusher.RunJSONLinesInDir(workDir, "drive", "push", "--json", "--force")
		require.NoError(t, err)
		assert.Equal(t, "uploaded", byPath(lines)["plans/ship.md"]["action"])
	})

	t.Run("push of an unpushable path fails the command", func(t *testing.T) {
		out, err := pusher.RunStdoutInDir(workDir, "drive", "push", "--json", "notes.txt")
		require.Error(t, err)
		// hive prints a failing command's error to stdout after the JSON.
		var line map[string]any
		require.NoError(t, json.NewDecoder(strings.NewReader(out)).Decode(&line))
		assert.Equal(t, "failed", line["action"])
		assert.NotEmpty(t, line["error"])
	})

	t.Run("ls marks uploaded and changed documents", func(t *testing.T) {
		out, err := pusher.RunStdoutInDir(workDir, "drive", "ls")
		require.NoError(t, err)
		assert.Regexp(t, `markdown\s+uploaded\s+plans/ship\.md`, out)

		require.NoError(t, os.WriteFile(filepath.Join(target, "plans", "ship.md"), []byte("# Ship v2\n"), 0o644))
		out, err = pusher.RunStdoutInDir(workDir, "drive", "ls")
		require.NoError(t, err)
		assert.Regexp(t, `markdown\s+changed\s+plans/ship\.md`, out)

		require.NoError(t, os.WriteFile(filepath.Join(target, "plans", "ship.md"), []byte("# Ship\n"), 0o644))
	})

	t.Run("pull into a second context root, then keep a differing file", func(t *testing.T) {
		puller := NewHarness(t).UseDriveServer(server)

		lines, err := puller.RunJSONLines("drive", "pull", "--repo", repo, "--json")
		require.NoError(t, err)
		got := byPath(lines)
		assert.Equal(t, "created", got["plans/ship.md"]["action"])
		assert.Equal(t, sha256Hex("# Ship\n"), got["plans/ship.md"]["sha256"])

		lines, err = puller.RunJSONLines("drive", "pull", "--repo", repo, "--json")
		require.NoError(t, err)
		assert.Equal(t, "unchanged", byPath(lines)["plans/ship.md"]["action"])

		out, err := puller.RunStdout("drive", "ls", "--repo", repo)
		require.NoError(t, err)
		spaceDir := strings.SplitN(out, "\n", 2)[0]
		local := filepath.Join(spaceDir, "plans", "ship.md")
		require.NoError(t, os.WriteFile(local, []byte("# Local edit\n"), 0o644))

		lines, err = puller.RunJSONLines("drive", "pull", "--repo", repo, "--json")
		require.NoError(t, err)
		got = byPath(lines)
		assert.Equal(t, "conflict", got["plans/ship.md"]["action"])
		assert.Equal(t, "plans/ship-1.md", got["plans/ship.md"]["local_path"])

		edited, err := os.ReadFile(local)
		require.NoError(t, err)
		assert.Equal(t, "# Local edit\n", string(edited), "pull never overwrites")
		copied, err := os.ReadFile(filepath.Join(spaceDir, "plans", "ship-1.md"))
		require.NoError(t, err)
		assert.Equal(t, "# Ship\n", string(copied))
	})

	t.Run("share prints a public link that serves the document", func(t *testing.T) {
		lines, err := pusher.RunJSONLinesInDir(workDir, "drive", "share", "plans/ship.md", "--expires", "7d", "--json")
		require.NoError(t, err)
		require.Len(t, lines, 1)
		assert.Equal(t, "shared", lines[0]["action"])
		url, _ := lines[0]["url"].(string)
		require.True(t, strings.HasPrefix(url, server.URL+"/s/"), url)
		assert.NotEmpty(t, lines[0]["expires_at"])

		resp, err := http.Get(url + "/raw")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "# Ship\n", string(body))
	})
}

func TestDriveWorkspaceFlag(t *testing.T) {
	h := NewHarness(t)
	out, err := h.Run("drive", "--workspace", "chats", "--shared", "ls")
	require.Error(t, err)
	assert.Contains(t, out, "--workspace cannot be combined")

	out, err = h.Run("drive", "-w", "chats", "-r", "a/b", "ls")
	require.Error(t, err)
	assert.Contains(t, out, "--workspace cannot be combined")
}
