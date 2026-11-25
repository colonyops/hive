package commands

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/hive/daemon/daemontest"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/hive/servers"
	"github.com/colonyops/hive/internal/ops/sessions"
)

const compatRemote = "https://github.com/acme/site"

// compatApp is a CLI app over a real engine whose fleet has server box, a
// test daemon reached without ssh.
func compatApp(t *testing.T) *app.App {
	t.Helper()
	t.Setenv("HIVE_SERVER", "")
	a := outputTestApp(t)
	socket := daemontest.Socket(t, hivetest.Mux{})
	dial := func(ctx context.Context, _ config.ServerConfig, _ io.Writer) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socket)
	}
	a.Servers = servers.New(zerolog.Nop(), a.SessionAPI(), []config.ServerConfig{{Name: "box", SSH: "box"}}, "test", servers.WithDialer(dial))
	t.Cleanup(a.Servers.Close)
	t.Chdir(t.TempDir())
	return a
}

func jsonLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "stdout line %q", line)
		lines = append(lines, m)
	}
	return lines
}

func runOK(t *testing.T, a *app.App, args ...string) outputRun {
	t.Helper()
	r := runForOutputTest(t, a, args...)
	require.Zero(t, r.code, "hive %s: %s%s", strings.Join(args, " "), r.reported, r.stderr)
	return r
}

func createCompat(t *testing.T, a *app.App, args ...string) map[string]any {
	t.Helper()
	r := runOK(t, a, append([]string{"session", "create", "--json", "--background", "--remote", compatRemote}, args...)...)
	lines := jsonLines(t, r.stdout)
	require.Len(t, lines, 1)
	return lines[0]
}

// The keys hive session show, create, update and recycle --json printed
// before the commands were generated.
var sessionJSONKeys = []string{"server", "id", "name", "slug", "repo", "remote", "path", "inbox", "state", "tags", "created_at", "updated_at"}

// The keys of a hive session list --json line before the commands were
// generated.
var listJSONKeys = []string{"server", "id", "name", "repo", "state", "tags"}

func assertKeys(t *testing.T, line map[string]any, keys []string) {
	t.Helper()
	for _, key := range keys {
		assert.Contains(t, line, key)
	}
}

func ids(lines []map[string]any) []string {
	var out []string
	for _, l := range lines {
		out = append(out, l["server"].(string)+":"+l["id"].(string))
	}
	return out
}

func TestSessionCommandsKeepTheirSpellings(t *testing.T) {
	a := compatApp(t)

	local := createCompat(t, a, "--tags", "auto", "worker", "one")
	assertKeys(t, local, sessionJSONKeys)
	assert.Equal(t, "local", local["server"])
	assert.Equal(t, "worker one", local["name"], "the name takes the rest of the words")
	assert.Equal(t, []any{"auto"}, local["tags"])
	localID := local["id"].(string)

	boxed := createCompat(t, a, "--server", "box", "remote")
	assert.Equal(t, "box", boxed["server"])
	boxID := boxed["id"].(string)

	short := runOK(t, a, "session", "create", "--json", "--bg", "-r", compatRemote, "short")
	assert.Equal(t, "short", jsonLines(t, short.stdout)[0]["name"])

	t.Run("session list --json", func(t *testing.T) {
		lines := jsonLines(t, runOK(t, a, "session", "list", "--json").stdout)
		assert.Subset(t, ids(lines), []string{"local:" + localID, "box:" + boxID})
		for _, line := range lines {
			assertKeys(t, line, listJSONKeys)
			if line["server"] == "local" {
				assert.Contains(t, line, "inbox")
				assert.Contains(t, line, "unread")
			} else {
				assert.NotContains(t, line, "inbox")
				assert.NotContains(t, line, "unread")
			}
		}
	})

	t.Run("session list --json --tags", func(t *testing.T) {
		lines := jsonLines(t, runOK(t, a, "session", "list", "--json", "--tags", "AUTO").stdout)
		assert.Equal(t, []string{"local:" + localID}, ids(lines))
	})

	t.Run("ls --json", func(t *testing.T) {
		assert.Equal(t,
			ids(jsonLines(t, runOK(t, a, "session", "list", "--json").stdout)),
			ids(jsonLines(t, runOK(t, a, "ls", "--json").stdout)))
	})

	t.Run("session show --json", func(t *testing.T) {
		for _, args := range [][]string{
			{"session", "show", localID, "--json"},
			{"session", "show", "local:" + localID, "--json"},
			{"session", "get", localID},
		} {
			lines := jsonLines(t, runOK(t, a, args...).stdout)
			require.Len(t, lines, 1, "%v", args)
			assertKeys(t, lines[0], sessionJSONKeys)
			assert.Equal(t, localID, lines[0]["id"])
		}
	})

	t.Run("server references", func(t *testing.T) {
		for _, args := range [][]string{
			{"session", "show", "box:" + boxID, "--json"},
			{"session", "show", "--server", "box", boxID, "--json"},
		} {
			lines := jsonLines(t, runOK(t, a, args...).stdout)
			require.Len(t, lines, 1)
			assert.Equal(t, "box", lines[0]["server"])
		}
		t.Setenv("HIVE_SERVER", "box")
		lines := jsonLines(t, runOK(t, a, "session", "show", boxID, "--json").stdout)
		assert.Equal(t, "box", lines[0]["server"], "HIVE_SERVER names the server of a bare id")
	})

	t.Run("session update --json", func(t *testing.T) {
		lines := jsonLines(t, runOK(t, a, "session", "update", "--json", "box:"+boxID, "--add-tag", "review", "--group", "web").stdout)
		assert.Equal(t, []any{"review"}, lines[0]["tags"])
		assert.Equal(t, "web", lines[0]["group"])
	})

	t.Run("session tags update on a server reference", func(t *testing.T) {
		lines := jsonLines(t, runOK(t, a, "session", "tags", "update", "box:"+boxID, "--add", "claimed", "--remove", "review").stdout)
		assert.Equal(t, []any{"claimed"}, lines[0]["tags"])
	})

	t.Run("session recycle", func(t *testing.T) {
		// The test executor clones nothing, so give the checkout the .git a
		// recycle checks for.
		require.NoError(t, os.MkdirAll(filepath.Join(local["path"].(string), ".git"), 0o755))
		r := runOK(t, a, "session", "recycle", localID)
		assert.Contains(t, r.stderr, "Session "+localID+" recycled")
		lines := jsonLines(t, r.stdout)
		require.Len(t, lines, 1)
		assert.Equal(t, localID, lines[0]["id"])
	})

	t.Run("session delete --json", func(t *testing.T) {
		lines := jsonLines(t, runOK(t, a, "session", "delete", "--json", "--force", "box:"+boxID).stdout)
		assert.Equal(t, map[string]any{"server": "box", "id": boxID, "deleted": true}, lines[0])
	})

	t.Run("unknown server", func(t *testing.T) {
		r := runForOutputTest(t, a, "session", "show", "nowhere:"+localID, "--json")
		assert.Equal(t, 3, r.code)
		var line errorLine
		require.NoError(t, json.Unmarshal([]byte(r.reported), &line))
		assert.Contains(t, line.Message, `unknown server "nowhere"`)
	})
}

func TestBatchKeepsItsResultJSON(t *testing.T) {
	a := compatApp(t)
	file := filepath.Join(t.TempDir(), "batch.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"sessions":[{"name":"batch one","remote":"`+compatRemote+`"}]}`), 0o600))

	r := runOK(t, a, "batch", "-f", file)
	var out struct {
		Results []struct {
			SessionID string `json:"session_id"`
			Path      string `json:"path"`
			Status    string `json:"status"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &out))
	require.Len(t, out.Results, 1)
	assert.NotEmpty(t, out.Results[0].SessionID)
	assert.NotEmpty(t, out.Results[0].Path)
	assert.Equal(t, "created", out.Results[0].Status)
}

// TestSessionListAnswersTheSameOverMCP fails when the session_list tool and
// hive session list --json list different sessions from one fleet.
func TestSessionListAnswersTheSameOverMCP(t *testing.T) {
	a := compatApp(t)
	createCompat(t, a, "here")
	createCompat(t, a, "--server", "box", "there")

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	for _, op := range engineops.New(zerolog.Nop(), a.Engine, engineops.WithFleet(func() *servers.Fleet { return a.Servers })).Ops() {
		if op.Spec.Info().Toolset == sessions.Toolset {
			op.Mount(srv, func(err error) error { return err })
		}
	}
	serverT, clientT := mcp.NewInMemoryTransports()
	_, err := srv.Connect(t.Context(), serverT, nil)
	require.NoError(t, err)
	client, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(t.Context(), clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	res, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "session_list", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, res.IsError, "%v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var listed sessions.SessionListOut
	require.NoError(t, json.Unmarshal(raw, &listed))
	var overMCP []string
	for _, s := range listed.Sessions {
		overMCP = append(overMCP, s.Server+":"+s.ID)
	}

	overCLI := ids(jsonLines(t, runOK(t, a, "session", "list", "--json").stdout))
	assert.Len(t, overCLI, 2)
	assert.Equal(t, overCLI, overMCP)
}
