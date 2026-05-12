//go:build unix

package timer_test

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/timer"
)

func TestForkChild(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}

	pid, err := timer.ForkChild(sleepPath, []string{"5"}, nil)
	if err != nil {
		t.Fatalf("ForkChild: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("ForkChild returned invalid PID %d", pid)
	}

	// Verify the child is alive: syscall.Kill(pid, 0) returns nil if it exists.
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("child PID %d should be alive immediately after fork: %v", pid, err)
	}

	// Clean up: signal the child to exit. We must reap it; since we released
	// the *os.Process we can't Wait, so just SIGKILL and move on. On Linux
	// the child becomes a zombie until reaped by init (PID 1 / launchd on
	// macOS adopts the session leader's orphans), which is fine for a test.
	_ = syscall.Kill(pid, syscall.SIGKILL)
	// Give the OS a moment so a follow-up `kill -0` in CI doesn't race.
	time.Sleep(50 * time.Millisecond)
}
