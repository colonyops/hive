//go:build unix

package timer

import (
	"fmt"
	"os/exec"
	"syscall"
)

// ForkChild starts the given executable detached from the current process
// group, so the child outlives the parent. The parent does NOT Wait — the
// caller is responsible for releasing the *os.Process if needed.
//
// `selfPath` is typically os.Executable(). `args` are passed to the child
// (the first element should be the subcommand name, e.g. "timer-fire").
// `env` is the child's environment (typically derived from os.Environ()
// plus any additional vars the child needs).
//
// Returns the child PID. The child's stdin/stdout/stderr are all closed.
//
// The setsid behavior creates a new session and process group for the
// child, fully detaching it from the parent's controlling terminal. This
// is what lets `hive timer-fire` survive the parent shell exiting.
func ForkChild(selfPath string, args []string, env []string) (int, error) {
	cmd := exec.Command(selfPath, args...)
	cmd.Env = env
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start detached child: %w", err)
	}

	// Capture the PID before Release — after Release, cmd.Process.Pid is
	// reset to -1 and the *os.Process must not be used.
	pid := cmd.Process.Pid

	// Release the child so the parent doesn't accumulate zombies.
	if err := cmd.Process.Release(); err != nil {
		// Already started; log-and-continue equivalent. Return the PID since
		// the child is alive.
		return pid, fmt.Errorf("release detached child: %w", err)
	}

	return pid, nil
}
