//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/multiplexer"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	tmuxadapter "github.com/colonyops/hive/internal/integration/multiplexer/tmux"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTmuxSessionCreated(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-repo")
	cleanupTmuxSession(t, "tmux-test")

	_, err := h.Run("new", "--remote", repo, "tmux-test")
	require.NoError(t, err)

	assertTmuxSessionExists(t, "tmux-test")
}

func TestTmuxWindows(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-win-repo")
	cleanupTmuxSession(t, "tmux-win-test")

	_, err := h.Run("new", "--remote", repo, "tmux-win-test")
	require.NoError(t, err)

	assertTmuxHasWindows(t, "tmux-win-test")
}

func TestTmuxRenameLifecycle(t *testing.T) {
	t.Run("success renames tmux and stores new target", func(t *testing.T) {
		h := NewHarness(t)
		repo := createBareRepo(t, "rename-success-repo")
		cleanupTmuxSession(t, "rename-success")
		cleanupTmuxSession(t, "rename-complete")
		_, err := h.Run("new", "--remote", repo, "rename-success")
		require.NoError(t, err)
		lines, err := h.RunJSONLines("ls", "--json")
		require.NoError(t, err)
		id := lines[0]["id"].(string)

		_, err = h.Run("session", "update", id, "--name", "rename-complete")
		require.NoError(t, err)
		assertTmuxSessionExists(t, "rename-complete")
		sess := integrationSession(t, h, id)
		assert.Equal(t, "rename-complete", sess.GetMeta(session.MetaTmuxSession))
	})

	t.Run("failure keeps old actual target in metadata", func(t *testing.T) {
		h := NewHarness(t)
		repo := createBareRepo(t, "rename-failure-repo")
		cleanupTmuxSession(t, "rename-failure")
		_, err := h.Run("new", "--remote", repo, "rename-failure")
		require.NoError(t, err)
		lines, err := h.RunJSONLines("ls", "--json")
		require.NoError(t, err)
		id := lines[0]["id"].(string)
		require.NoError(t, exec.Command("tmux", "kill-session", "-t", "rename-failure").Run())

		_, err = h.Run("session", "update", id, "--name", "renamed-without-tmux")
		require.NoError(t, err)
		sess := integrationSession(t, h, id)
		assert.Equal(t, "renamed-without-tmux", sess.Name)
		assert.Equal(t, "rename-failure", sess.GetMeta(session.MetaTmuxSession))
	})
}

func TestTmuxMetadataTargetUsedForRecycleAndDelete(t *testing.T) {
	for _, operation := range []string{"recycle", "delete"} {
		t.Run(operation, func(t *testing.T) {
			h := NewHarness(t)
			repo := createBareRepo(t, "metadata-"+operation+"-repo")
			slug := "metadata-" + operation
			actual := slug + "-actual"
			cleanupTmuxSession(t, slug)
			cleanupTmuxSession(t, actual)
			_, err := h.Run("new", "--remote", repo, slug)
			require.NoError(t, err)
			lines, err := h.RunJSONLines("ls", "--json")
			require.NoError(t, err)
			id := lines[0]["id"].(string)
			out, err := exec.Command("tmux", "rename-session", "-t", slug, actual).CombinedOutput()
			require.NoError(t, err, "tmux rename-session: %s", out)
			setIntegrationTmuxTarget(t, h, id, actual)

			_, err = h.Run("session", operation, id)
			require.NoError(t, err)
			err = exec.Command("tmux", "has-session", "-t", actual).Run()
			assert.Error(t, err, "metadata target should be killed")
		})
	}
}

func TestTmuxTypedWindowKillAndMissingTargets(t *testing.T) {
	client := tmuxadapter.NewDefault(zerolog.Nop())
	cleanupTmuxSession(t, "window-lifecycle")
	out, err := exec.Command("tmux", "new-session", "-d", "-s", "window-lifecycle", "-n", "one").CombinedOutput()
	require.NoError(t, err, "tmux new-session: %s", out)
	out, err = exec.Command("tmux", "new-window", "-t", "window-lifecycle", "-n", "two").CombinedOutput()
	require.NoError(t, err, "tmux new-window: %s", out)

	require.NoError(t, client.KillWindow(context.Background(), multiplexer.Target{Session: "window-lifecycle", Window: "two"}))
	windows, err := exec.Command("tmux", "list-windows", "-t", "window-lifecycle", "-F", "#{window_name}").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "one", strings.TrimSpace(string(windows)))
	assert.Error(t, client.KillWindow(context.Background(), multiplexer.Target{Session: "window-lifecycle", Window: "missing"}))
	assert.Error(t, client.KillSession(context.Background(), multiplexer.Target{Session: "missing-session"}))
}

func integrationSession(t *testing.T, h *Harness, id string) session.Session {
	t.Helper()
	database, err := db.Open(h.DataDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, database.Close()) }()
	sess, err := stores.NewSessionStore(database).Get(context.Background(), id)
	require.NoError(t, err)
	return sess
}

func setIntegrationTmuxTarget(t *testing.T, h *Harness, id, target string) {
	t.Helper()
	database, err := db.Open(h.DataDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, database.Close()) }()
	store := stores.NewSessionStore(database)
	sess, err := store.Get(context.Background(), id)
	require.NoError(t, err)
	sess.SetMeta(session.MetaTmuxSession, target)
	require.NoError(t, store.Save(context.Background(), sess))
}

func TestTmuxCapture(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-cap-repo")
	cleanupTmuxSession(t, "tmux-cap-test")

	_, err := h.Run("new", "--remote", repo, "tmux-cap-test")
	require.NoError(t, err)

	assertTmuxSessionExists(t, "tmux-cap-test")

	// Send a known command and verify it appears in the captured pane output.
	_, err = exec.Command("tmux", "send-keys", "-t", "tmux-cap-test", "echo hive-capture-test", "Enter").CombinedOutput()
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := exec.Command("tmux", "capture-pane", "-t", "tmux-cap-test", "-p").CombinedOutput()
		assert.NoError(c, err, "tmux capture-pane: %s", out)
		assert.Contains(c, string(out), "hive-capture-test")
	}, 5*time.Second, 200*time.Millisecond)
}

func TestTmuxInputPrimitives(t *testing.T) {
	ctx := context.Background()
	client := tmuxadapter.NewDefault(zerolog.Nop())
	target := multiplexer.Target{Session: "input-primitives", Window: "0", Pane: "0"}
	cleanupTmuxSession(t, target.Session)
	outputPath := filepath.Join(t.TempDir(), "literal.bin")
	fixture := "--quotes ' double\"; unicode λ\tkey-looking C-c"

	out, err := exec.Command("tmux", "new-session", "-d", "-s", target.Session, fmt.Sprintf("cat > %q", outputPath)).CombinedOutput()
	require.NoError(t, err, "tmux new-session: %s", out)

	require.NoError(t, client.SendLiteral(ctx, target, fixture))
	enter, err := multiplexer.NewNamedKey("Enter")
	require.NoError(t, err)
	require.NoError(t, client.SendKey(ctx, target, enter))
	down, err := multiplexer.NewNamedKey("Down")
	require.NoError(t, err)
	require.NoError(t, client.SendKey(ctx, target, down))
	require.NoError(t, client.SendKey(ctx, target, enter))
	interrupt, err := multiplexer.NewNamedKey("C-c")
	require.NoError(t, err)
	require.NoError(t, client.SendKey(ctx, target, interrupt))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		got, readErr := os.ReadFile(outputPath)
		assert.NoError(c, readErr)
		assert.Equal(c, fixture+"\n\x1b[B\n", string(got))
	}, 5*time.Second, 100*time.Millisecond)
	require.Eventually(t, func() bool {
		return exec.Command("tmux", "has-session", "-t", target.Session).Run() != nil
	}, 5*time.Second, 100*time.Millisecond, "C-c should terminate the receiver session")
}

func TestTmuxPasteIsByteExactAndCleansBuffer(t *testing.T) {
	ctx := context.Background()
	client := tmuxadapter.NewDefault(zerolog.Nop())
	target := multiplexer.Target{Session: "paste-primitives", Window: "0", Pane: "0"}
	cleanupTmuxSession(t, target.Session)
	outputPath := filepath.Join(t.TempDir(), "paste.bin")
	fixture := []byte("first line\n--second\tλ\nthird; 'quoted'\n")
	command := fmt.Sprintf("dd bs=1 count=%d of=%q 2>/dev/null", len(fixture), outputPath)

	out, err := exec.Command("tmux", "new-session", "-d", "-s", target.Session, command).CombinedOutput()
	require.NoError(t, err, "tmux new-session: %s", out)
	require.NoError(t, client.Paste(ctx, target, fixture, multiplexer.PasteOptions{}))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		got, readErr := os.ReadFile(outputPath)
		assert.NoError(c, readErr)
		assert.Equal(c, fixture, got)
	}, 5*time.Second, 100*time.Millisecond)

	buffers, _ := exec.Command("tmux", "list-buffers", "-F", "#{buffer_name}").CombinedOutput()
	for name := range strings.SplitSeq(strings.TrimSpace(string(buffers)), "\n") {
		assert.False(t, strings.HasPrefix(name, "hive-"), "leftover buffer %q", name)
	}
}

func TestAssessScenarioSendsLiteralTextAndEnter(t *testing.T) {
	h := NewHarness(t)
	cleanupTmuxSession(t, "scenario-input")
	outputPath := filepath.Join(t.TempDir(), "input.txt")
	scenarioPath := filepath.Join(t.TempDir(), "scenario.yaml")
	fixture := "--quotes ' double\"; unicode λ key-looking C-c"
	scenario := "scenario: literal input\nsteps:\n  - send: " + fmt.Sprintf("%q", fixture) + "\n"
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenario), 0o600))

	out, err := exec.Command("tmux", "new-session", "-d", "-s", "scenario-input", "cat > "+outputPath).CombinedOutput()
	require.NoError(t, err, "tmux new-session: %s", out)

	_, err = h.RunStdout("x", "assess", "scenario", scenarioPath, "--target", "scenario-input:0.0", "--allow-host")
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		got, readErr := os.ReadFile(outputPath)
		assert.NoError(c, readErr)
		assert.Equal(c, fixture+"\n", string(got))
	}, 5*time.Second, 100*time.Millisecond)
}

func TestSpawnConfigs(t *testing.T) {
	type spawnConfigCase struct {
		name        string
		config      string
		wantSession string
		wantWindows []string // nil = skip window name assertion
	}

	cases := []spawnConfigCase{
		{
			name: "spawn commands",
			config: `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
`,
			wantSession: "spawn-cmd",
			wantWindows: nil, // spawn manages tmux directly; window name is unpredictable
		},
		{
			name: "declarative windows",
			config: `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - windows:
      - name: agent
        command: bash
      - name: shell
`,
			wantSession: "decl-win",
			wantWindows: []string{"agent", "shell"},
		},
		{
			name: "default no rules",
			config: `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules: []
`,
			wantSession: "def-norule",
			wantWindows: []string{"testbash", "shell"},
		},
		{
			name: "pattern no match falls through to defaults",
			config: `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - pattern: "^https://github\\.com/never-match/"
    windows:
      - name: custom
        command: bash
`,
			wantSession: "pat-nomatch",
			wantWindows: []string{"testbash", "shell"},
		},
		{
			name: "windows with focus",
			config: `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - windows:
      - name: code
        command: bash
      - name: runner
        command: bash
        focus: true
`,
			wantSession: "win-focus",
			wantWindows: []string{"code", "runner"},
		},
		{
			name: "windows single command only",
			config: `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - windows:
      - name: work
        command: bash
`,
			wantSession: "win-single",
			wantWindows: []string{"work"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHarness(t).WithConfig(tc.config)
			repo := createBareRepo(t, "spawn-cfg-"+tc.wantSession)
			cleanupTmuxSession(t, tc.wantSession)

			out, err := h.Run("new", "--background", "--remote", repo, tc.wantSession)
			if err != nil {
				logPath := filepath.Join(h.DataDir(), "hive.log")
				logData, _ := os.ReadFile(logPath)
				t.Fatalf("hive new failed: %v\noutput: %s\nlog: %s", err, out, logData)
			}

			assertTmuxSessionExists(t, tc.wantSession)

			if tc.wantWindows != nil {
				assertTmuxWindowNames(t, tc.wantSession, tc.wantWindows)
			}
		})
	}
}

func TestTmuxWindowPanes(t *testing.T) {
	config := `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - windows:
      - name: agent
        focus: true
        panes:
          - command: bash
          - command: bash
            split: horizontal
            size: 30%
      - name: shell
`
	h := NewHarness(t).WithConfig(config)
	repo := createBareRepo(t, "tmux-pane-repo")
	cleanupTmuxSession(t, "tmux-pane-test")

	_, err := h.Run("new", "--background", "--remote", repo, "tmux-pane-test")
	require.NoError(t, err)

	assertTmuxWindowNames(t, "tmux-pane-test", []string{"agent", "shell"})
	assertTmuxPaneCount(t, "tmux-pane-test:agent", 2)
	assertTmuxPaneCount(t, "tmux-pane-test:shell", 1)

	active, err := exec.Command("tmux", "display-message", "-p", "-t", "tmux-pane-test", "#{window_name}").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "agent", strings.TrimSpace(string(active)))

	lines, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	sess := integrationSession(t, h, lines[0]["id"].(string))
	sessionPath := sess.Path
	paneRows, err := exec.Command("tmux", "list-panes", "-t", "tmux-pane-test:agent", "-F", "#{pane_current_path}|||#{pane_width}|||#{pane_height}").CombinedOutput()
	require.NoError(t, err)
	rows := strings.Split(strings.TrimSpace(string(paneRows)), "\n")
	require.Len(t, rows, 2)
	first := strings.Split(rows[0], "|||")
	second := strings.Split(rows[1], "|||")
	require.Len(t, first, 3)
	require.Len(t, second, 3)
	assert.Equal(t, sessionPath, first[0])
	assert.Equal(t, sessionPath, second[0])
	assert.NotEqual(t, first[1], second[1], "30%% horizontal split should create unequal pane widths")
	assert.Equal(t, first[2], second[2], "horizontal split should preserve pane height")
}

func TestTmuxCreateSessionCleansPartialSession(t *testing.T) {
	client := tmuxadapter.NewDefault(zerolog.Nop())
	name := "partial-cleanup"
	cleanupTmuxSession(t, name)

	err := client.CreateSession(context.Background(), multiplexer.SessionSpec{
		Target:     multiplexer.Target{Session: name},
		Background: true,
		Windows: []multiplexer.WindowSpec{{
			Name: "created",
			Panes: []multiplexer.PaneSpec{
				{Command: "sleep 600"},
				{Command: "sleep 600", Size: "not-a-size"},
			},
		}},
	})
	require.Error(t, err)
	assert.Error(t, exec.Command("tmux", "has-session", "-t", name).Run(), "partial session should be removed")
}

func TestTmuxListAll(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-list-repo")
	cleanupTmuxSession(t, "tmux-list-a")
	cleanupTmuxSession(t, "tmux-list-b")

	_, err := h.Run("new", "--remote", repo, "tmux-list-a")
	require.NoError(t, err)
	_, err = h.Run("new", "--remote", repo, "tmux-list-b")
	require.NoError(t, err)

	assertTmuxSessionExists(t, "tmux-list-a", "tmux-list-b")

	lsOut, err := h.Run("ls")
	require.NoError(t, err)
	assert.Contains(t, lsOut, "tmux-list-a")
	assert.Contains(t, lsOut, "tmux-list-b")
}
