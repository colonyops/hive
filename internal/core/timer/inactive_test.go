//go:build unix

package timer_test

import (
	"context"
	"database/sql"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/timer"
	"github.com/colonyops/hive/internal/data/db"
)

// dbAdapter wraps *db.Queries to satisfy timer.InactiveQuerier. It exists
// because timer cannot import db (db imports timer for the Status enum), so
// the helper takes a small interface and tests provide this adapter.
type dbAdapter struct{ q *db.Queries }

func (a dbAdapter) ActiveTimersForSession(ctx context.Context, sessionID string) ([]timer.Row, error) {
	rows, err := a.q.ActiveTimersForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return toRows(rows), nil
}

func (a dbAdapter) ActiveTimersAll(ctx context.Context) ([]timer.Row, error) {
	rows, err := a.q.ActiveTimersAll(ctx)
	if err != nil {
		return nil, err
	}
	return toRows(rows), nil
}

func (a dbAdapter) MarkInactiveTimersForSession(ctx context.Context, arg timer.MarkInactiveParams) error {
	return a.q.MarkInactiveTimersForSession(ctx, db.MarkInactiveTimersForSessionParams{
		SessionID: arg.SessionID,
		Ids:       arg.IDs,
	})
}

func (a dbAdapter) MarkInactiveTimersAll(ctx context.Context, ids []string) error {
	return a.q.MarkInactiveTimersAll(ctx, ids)
}

func toRows(in []db.Timer) []timer.Row {
	out := make([]timer.Row, len(in))
	for i, r := range in {
		out[i] = timer.Row{ID: r.ID, Pid: r.Pid}
	}
	return out
}

// openTestDB opens an isolated DB in t.TempDir() and registers cleanup.
func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("db close: %v", err)
		}
	})
	return database
}

// spawnLiveSleep starts a long-running sleep process and returns its PID.
// The process is SIGKILLed during cleanup.
func spawnLiveSleep(t *testing.T) int {
	t.Helper()
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	cmd := exec.Command(sleepPath, "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})
	return pid
}

// spawnDeadPID runs sleep 0.05 to completion and returns its (now-dead) PID.
// Waits an additional 100ms to make sure the kernel marked it gone.
func spawnDeadPID(t *testing.T) int {
	t.Helper()
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	cmd := exec.Command(sleepPath, "0.05")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait sleep: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	// Sanity: confirm dead.
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("expected PID %d to be dead, kill(0) returned nil", pid)
	}
	return pid
}

func insertTimer(t *testing.T, ctx context.Context, q *db.Queries, id, sessionID string, pid sql.NullInt64, status timer.Status) {
	t.Helper()
	if err := q.InsertTimer(ctx, db.InsertTimerParams{
		ID:         id,
		SessionID:  sessionID,
		TmuxTarget: "tmux-target",
		Prompt:     "prompt",
		DurationNs: int64(time.Minute),
		FiresAt:    time.Now().Add(time.Minute).UnixNano(),
		Pid:        pid,
		Status:     status,
		CreatedAt:  time.Now().UnixNano(),
		FiredAt:    sql.NullInt64{},
	}); err != nil {
		t.Fatalf("insert timer %q: %v", id, err)
	}
}

func TestMarkInactiveForSession_HappyPath(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	q := database.Queries()
	adapter := dbAdapter{q: q}

	livePID := spawnLiveSleep(t)
	deadPID := spawnDeadPID(t)

	insertTimer(t, ctx, q, "t-live", "s1", sql.NullInt64{Int64: int64(livePID), Valid: true}, timer.StatusActive)
	insertTimer(t, ctx, q, "t-dead", "s1", sql.NullInt64{Int64: int64(deadPID), Valid: true}, timer.StatusActive)
	insertTimer(t, ctx, q, "t-nullpid", "s1", sql.NullInt64{}, timer.StatusActive)

	n, err := timer.MarkInactiveForSession(ctx, adapter, "s1")
	if err != nil {
		t.Fatalf("MarkInactiveForSession: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row transitioned, got %d", n)
	}

	got, err := q.GetTimer(ctx, "t-dead")
	if err != nil {
		t.Fatalf("get t-dead: %v", err)
	}
	if got.Status != timer.StatusOrphaned {
		t.Fatalf("t-dead status: want %q, got %q", timer.StatusOrphaned, got.Status)
	}

	got, err = q.GetTimer(ctx, "t-live")
	if err != nil {
		t.Fatalf("get t-live: %v", err)
	}
	if got.Status != timer.StatusActive {
		t.Fatalf("t-live status: want %q, got %q", timer.StatusActive, got.Status)
	}

	got, err = q.GetTimer(ctx, "t-nullpid")
	if err != nil {
		t.Fatalf("get t-nullpid: %v", err)
	}
	if got.Status != timer.StatusActive {
		t.Fatalf("t-nullpid status: want %q, got %q", timer.StatusActive, got.Status)
	}
}

func TestMarkInactiveForSession_OtherSessionIsolated(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	q := database.Queries()
	adapter := dbAdapter{q: q}

	deadPID := spawnDeadPID(t)
	insertTimer(t, ctx, q, "t-s2-dead", "s2", sql.NullInt64{Int64: int64(deadPID), Valid: true}, timer.StatusActive)

	n, err := timer.MarkInactiveForSession(ctx, adapter, "s1")
	if err != nil {
		t.Fatalf("MarkInactiveForSession: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows transitioned (other session), got %d", n)
	}

	got, err := q.GetTimer(ctx, "t-s2-dead")
	if err != nil {
		t.Fatalf("get t-s2-dead: %v", err)
	}
	if got.Status != timer.StatusActive {
		t.Fatalf("t-s2-dead status: want active (other session untouched), got %q", got.Status)
	}
}

func TestMarkInactiveAll(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	q := database.Queries()
	adapter := dbAdapter{q: q}

	deadA := spawnDeadPID(t)
	deadB := spawnDeadPID(t)
	livePID := spawnLiveSleep(t)

	insertTimer(t, ctx, q, "ta", "s1", sql.NullInt64{Int64: int64(deadA), Valid: true}, timer.StatusActive)
	insertTimer(t, ctx, q, "tb", "s2", sql.NullInt64{Int64: int64(deadB), Valid: true}, timer.StatusActive)
	insertTimer(t, ctx, q, "tc", "s3", sql.NullInt64{Int64: int64(livePID), Valid: true}, timer.StatusActive)

	n, err := timer.MarkInactiveAll(ctx, adapter)
	if err != nil {
		t.Fatalf("MarkInactiveAll: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows transitioned, got %d", n)
	}

	for _, id := range []string{"ta", "tb"} {
		got, err := q.GetTimer(ctx, id)
		if err != nil {
			t.Fatalf("get %q: %v", id, err)
		}
		if got.Status != timer.StatusOrphaned {
			t.Fatalf("%s status: want %q, got %q", id, timer.StatusOrphaned, got.Status)
		}
	}

	got, err := q.GetTimer(ctx, "tc")
	if err != nil {
		t.Fatalf("get tc: %v", err)
	}
	if got.Status != timer.StatusActive {
		t.Fatalf("tc status: want active, got %q", got.Status)
	}
}

func TestMarkInactive_NoActiveRows(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	adapter := dbAdapter{q: database.Queries()}

	nSess, err := timer.MarkInactiveForSession(ctx, adapter, "s1")
	if err != nil {
		t.Fatalf("MarkInactiveForSession: %v", err)
	}
	if nSess != 0 {
		t.Fatalf("expected 0 for empty session, got %d", nSess)
	}

	nAll, err := timer.MarkInactiveAll(ctx, adapter)
	if err != nil {
		t.Fatalf("MarkInactiveAll: %v", err)
	}
	if nAll != 0 {
		t.Fatalf("expected 0 for empty DB, got %d", nAll)
	}
}
