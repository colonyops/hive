//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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

func TestTmuxCapture(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-cap-repo")
	cleanupTmuxSession(t, "tmux-cap-test")

	_, err := h.Run("new", "--remote", repo, "tmux-cap-test")
	require.NoError(t, err)

	assertTmuxSessionExists(t, "tmux-cap-test")

	out, err := exec.Command("tmux", "capture-pane", "-t", "tmux-cap-test", "-p").CombinedOutput()
	require.NoError(t, err, "tmux capture-pane: %s", out)
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
