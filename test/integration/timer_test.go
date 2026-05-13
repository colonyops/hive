//go:build integration

package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

// openTimerDB opens the harness's hive.db read-only for direct queries.
func openTimerDB(t *testing.T, h *Harness) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(h.DataDir(), "hive.db")
	sqlDB, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(2000)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

// killActiveTimerChildren reads the active-timer PIDs from the DB and
// SIGKILLs each. Registered via t.Cleanup so long-duration test timers
// don't leak processes after the test completes.
func killActiveTimerChildren(t *testing.T, h *Harness) {
	t.Helper()
	dbPath := filepath.Join(h.DataDir(), "hive.db")
	if _, err := os.Stat(dbPath); err != nil {
		return
	}
	sqlDB, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(2000)")
	if err != nil {
		t.Logf("open db for cleanup: %v", err)
		return
	}
	defer sqlDB.Close()
	rows, err := sqlDB.Query("SELECT pid FROM timers WHERE status='active' AND pid IS NOT NULL")
	if err != nil {
		t.Logf("query active timer pids: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pid sql.NullInt64
		if err := rows.Scan(&pid); err != nil {
			continue
		}
		if pid.Valid {
			_ = exec.Command("kill", "-9", strconv.FormatInt(pid.Int64, 10)).Run()
		}
	}
}

// readJSONLogLines parses <dataDir>/hive.log line-by-line as JSON objects.
// Non-JSON lines are skipped (defensive — should never happen now that the
// logger emits structured JSON to files).
func readJSONLogLines(t *testing.T, h *Harness) []map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(h.DataDir(), "hive.log"))
	if err != nil {
		t.Fatalf("read hive.log: %v", err)
	}
	var lines []map[string]any
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		lines = append(lines, obj)
	}
	return lines
}

// waitTimerStatus polls the timers row until status matches `want` (or the
// status row reaches a terminal state) or the deadline elapses.
func waitTimerStatus(t *testing.T, h *Harness, id, want string, timeout time.Duration) string {
	t.Helper()
	sqlDB := openTimerDB(t, h)
	deadline := time.Now().Add(timeout)
	var got string
	for time.Now().Before(deadline) {
		err := sqlDB.QueryRow("SELECT status FROM timers WHERE id = ?", id).Scan(&got)
		if err == nil && got == want {
			return got
		}
		time.Sleep(100 * time.Millisecond)
	}
	return got
}

func TestTimerFiresIntoPane(t *testing.T) {
	h := NewHarness(t)
	t.Cleanup(func() { killActiveTimerChildren(t, h) })

	repo := createBareRepo(t, "timer-fire-repo")
	cleanupTmuxSession(t, "timer-fire-test")

	out, err := h.Run("new", "--remote", repo, "timer-fire-test")
	require.NoError(t, err, "hive new failed: %s", out)
	assertTmuxSessionExists(t, "timer-fire-test")

	dir := sessionDir(t, h)
	scheduleOut, err := h.RunStdoutInDir(dir, "timer", "-d", "2s", "-p", "hive-timer-fired-marker", "--json")
	require.NoError(t, err, "timer schedule failed: %s", scheduleOut)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(scheduleOut)), &result), "parse JSON: %s", scheduleOut)
	id, _ := result["id"].(string)
	firesAt, _ := result["fires_at"].(string)
	require.NotEmpty(t, id, "id missing in: %s", scheduleOut)
	require.NotEmpty(t, firesAt, "fires_at missing in: %s", scheduleOut)

	// Verify the marker appears in the tmux pane after the timer fires.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		paneOut, err := exec.Command("tmux", "capture-pane", "-t", "timer-fire-test", "-p").CombinedOutput()
		assert.NoError(c, err, "tmux capture-pane: %s", paneOut)
		assert.Contains(c, string(paneOut), "hive-timer-fired-marker")
	}, 8*time.Second, 250*time.Millisecond)

	// Verify the DB row was marked fired.
	final := waitTimerStatus(t, h, id, "fired", 3*time.Second)
	require.Equal(t, "fired", final, "expected status=fired for %s", id)

	sqlDB := openTimerDB(t, h)
	var status string
	var firedAt sql.NullInt64
	require.NoError(t, sqlDB.QueryRow("SELECT status, fired_at FROM timers WHERE id = ?", id).Scan(&status, &firedAt))
	assert.Equal(t, "fired", status)
	assert.True(t, firedAt.Valid, "fired_at should be set")
}

func TestTimerCapRejection(t *testing.T) {
	h := NewHarness(t)
	t.Cleanup(func() { killActiveTimerChildren(t, h) })

	repo := createBareRepo(t, "timer-cap-repo")
	cleanupTmuxSession(t, "cap-test")

	_, err := h.Run("new", "--remote", repo, "cap-test")
	require.NoError(t, err)
	assertTmuxSessionExists(t, "cap-test")

	dir := sessionDir(t, h)

	// Schedule 3 long-running timers — each should succeed.
	for i := 0; i < 3; i++ {
		out, err := h.RunInDir(dir, "timer", "-d", "1h", "-p", fmt.Sprintf("p%d", i))
		require.NoError(t, err, "schedule %d failed: %s", i, out)
	}

	// 4th without --ignore-limit: must fail.
	out, err := h.RunInDir(dir, "timer", "-d", "1h", "-p", "p4")
	require.Error(t, err, "expected 4th timer to be rejected; output: %s", out)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "expected exit error, got: %T %v", err, err)
	assert.Equal(t, 1, exitErr.ExitCode(), "expected exit code 1")
	lower := strings.ToLower(out)
	assert.True(t,
		strings.Contains(lower, "cap") || strings.Contains(lower, "ignore-limit"),
		"expected error to mention 'cap' or 'ignore-limit'; got: %s", out)

	// 4th with --ignore-limit: must succeed.
	out, err = h.RunInDir(dir, "timer", "-d", "1h", "-p", "p4-bypass", "--ignore-limit")
	require.NoError(t, err, "bypass should succeed: %s", out)

	// Exactly one log line should report the cap bypass with session_id.
	logs := readJSONLogLines(t, h)
	bypassCount := 0
	for _, line := range logs {
		msg, _ := line["message"].(string)
		if msg != "timer cap bypass" {
			continue
		}
		sid, _ := line["session_id"].(string)
		assert.NotEmpty(t, sid, "expected session_id on bypass log line: %v", line)
		bypassCount++
	}
	assert.Equal(t, 1, bypassCount, "expected exactly one 'timer cap bypass' log line")
}

func TestTimerValidationRejection(t *testing.T) {
	h := NewHarness(t)
	t.Cleanup(func() { killActiveTimerChildren(t, h) })

	repo := createBareRepo(t, "timer-validate-repo")
	cleanupTmuxSession(t, "validate-test")

	_, err := h.Run("new", "--remote", repo, "validate-test")
	require.NoError(t, err)
	assertTmuxSessionExists(t, "validate-test")

	dir := sessionDir(t, h)

	type vCase struct {
		name      string
		args      []string
		wantInMsg []string // substrings expected in the error output (any one)
	}

	bigPrompt := strings.Repeat("a", 8193)
	cases := []vCase{
		{
			name:      "duration below minimum",
			args:      []string{"timer", "-d", "500ms", "-p", "x"},
			wantInMsg: []string{"minimum", "below"},
		},
		{
			name:      "duration zero",
			args:      []string{"timer", "-d", "0", "-p", "x"},
			wantInMsg: []string{"minimum", "below", "invalid"},
		},
		{
			name:      "duration negative",
			args:      []string{"timer", "-d", "-5s", "-p", "x"},
			wantInMsg: []string{"minimum", "below", "invalid"},
		},
		{
			name:      "duration above maximum",
			args:      []string{"timer", "-d", "5h", "-p", "x"},
			wantInMsg: []string{"maximum", "exceeds"},
		},
		{
			name:      "duration unparseable",
			args:      []string{"timer", "-d", "abc", "-p", "x"},
			wantInMsg: []string{"invalid duration"},
		},
		{
			name:      "prompt empty",
			args:      []string{"timer", "-d", "5s", "-p", ""},
			wantInMsg: []string{"empty", "prompt"},
		},
		{
			name:      "prompt too large",
			args:      []string{"timer", "-d", "5s", "-p", bigPrompt},
			wantInMsg: []string{"maximum", "8192"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := h.RunInDir(dir, tc.args...)
			require.Error(t, err, "expected error for %q; got output: %s", tc.name, out)
			exitErr, ok := err.(*exec.ExitError)
			require.True(t, ok, "expected exit error; got %T %v", err, err)
			assert.Equal(t, 1, exitErr.ExitCode(), "exit code")
			lower := strings.ToLower(out)
			matched := false
			for _, want := range tc.wantInMsg {
				if strings.Contains(lower, strings.ToLower(want)) {
					matched = true
					break
				}
			}
			assert.True(t, matched, "expected error to contain one of %v; got: %s", tc.wantInMsg, out)
		})
	}
}

func TestTimerDetectSessionRejection(t *testing.T) {
	h := NewHarness(t)
	t.Cleanup(func() { killActiveTimerChildren(t, h) })

	// No hive session created; run from a temp dir outside any repo.
	tmp := t.TempDir()
	out, err := h.RunInDir(tmp, "timer", "-d", "5s", "-p", "x")
	require.Error(t, err, "expected error when no session detected; output: %s", out)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "expected exit error; got %T %v", err, err)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, strings.ToLower(out), "session", "expected 'session' in error: %s", out)
}

func TestTimerMarksOrphan(t *testing.T) {
	h := NewHarness(t)
	t.Cleanup(func() { killActiveTimerChildren(t, h) })

	repo := createBareRepo(t, "timer-orphan-repo")
	cleanupTmuxSession(t, "orphan-test")

	_, err := h.Run("new", "--remote", repo, "orphan-test")
	require.NoError(t, err)
	assertTmuxSessionExists(t, "orphan-test")

	dir := sessionDir(t, h)

	// Schedule first long timer and capture its PID.
	out, err := h.RunStdoutInDir(dir, "timer", "-d", "1h", "-p", "first", "--json")
	require.NoError(t, err, "first schedule failed: %s", out)
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &first), "parse JSON: %s", out)
	firstID, _ := first["id"].(string)
	pidFloat, _ := first["pid"].(float64)
	require.NotZero(t, pidFloat, "expected non-zero pid: %s", out)

	// Kill the detached child.
	require.NoError(t, exec.Command("kill", "-9", strconv.Itoa(int(pidFloat))).Run())
	time.Sleep(300 * time.Millisecond)

	// Schedule a second timer in the same session — should trigger the
	// schedule-time inactive-marker pass and orphan the first row.
	out2, err := h.RunStdoutInDir(dir, "timer", "-d", "1h", "-p", "second", "--json")
	require.NoError(t, err, "second schedule failed: %s", out2)
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out2)), &second))
	secondID, _ := second["id"].(string)

	sqlDB := openTimerDB(t, h)
	rows, err := sqlDB.Query("SELECT id, status FROM timers ORDER BY created_at ASC")
	require.NoError(t, err)
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var id, status string
		require.NoError(t, rows.Scan(&id, &status))
		got[id] = status
	}
	assert.Equal(t, "orphaned", got[firstID], "first timer should be orphaned; got=%v", got)
	assert.Equal(t, "active", got[secondID], "second timer should be active; got=%v", got)
}

func TestTimerTmuxSessionRenamed(t *testing.T) {
	h := NewHarness(t)
	t.Cleanup(func() { killActiveTimerChildren(t, h) })

	repo := createBareRepo(t, "timer-rename-repo")
	cleanupTmuxSession(t, "rename-test")
	cleanupTmuxSession(t, "renamed-mid-test")

	_, err := h.Run("new", "--remote", repo, "rename-test")
	require.NoError(t, err)
	assertTmuxSessionExists(t, "rename-test")

	dir := sessionDir(t, h)

	out, err := h.RunStdoutInDir(dir, "timer", "-d", "2s", "-p", "after-rename", "--json")
	require.NoError(t, err, "schedule failed: %s", out)
	var sched map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &sched))
	id, _ := sched["id"].(string)
	require.NotEmpty(t, id)

	// Rename the tmux session out from under the child before it fires.
	renameOut, renameErr := exec.Command("tmux", "rename-session", "-t", "rename-test", "renamed-mid-test").CombinedOutput()
	require.NoError(t, renameErr, "tmux rename-session: %s", renameOut)

	// Wait for the timer to fire (and fail).
	final := waitTimerStatus(t, h, id, "failed", 8*time.Second)
	assert.Equal(t, "failed", final, "expected status=failed for %s", id)

	// Verify exactly one structured fire-failure log line with session_exists=false.
	logs := readJSONLogLines(t, h)
	failCount := 0
	for _, line := range logs {
		msg, _ := line["message"].(string)
		if msg != "timer fire failed" {
			continue
		}
		failCount++
		// session_exists is encoded as a bool in zerolog JSON.
		se, ok := line["session_exists"].(bool)
		assert.True(t, ok, "session_exists field should be bool: %v", line)
		assert.False(t, se, "session_exists should be false after rename: %v", line)
	}
	assert.Equal(t, 1, failCount, "expected exactly one 'timer fire failed' log line; got logs: %d lines", len(logs))
}
