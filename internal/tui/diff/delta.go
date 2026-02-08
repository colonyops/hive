package diff

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// DeltaNotFoundError is returned when delta is not installed.
type DeltaNotFoundError struct{}

func (e *DeltaNotFoundError) Error() string {
	return `delta not found in PATH

Delta is required for syntax highlighting in the diff viewer.
Install it with:

  brew install git-delta           # macOS
  cargo install git-delta          # Cargo
  sudo apt install git-delta       # Debian/Ubuntu

Or visit: https://github.com/dandavison/delta`
}

// CheckDeltaAvailable checks if delta is installed and available in PATH.
// Returns DeltaNotFoundError if delta is not found.
func CheckDeltaAvailable() error {
	_, err := exec.LookPath("delta")
	if err != nil {
		return &DeltaNotFoundError{}
	}
	return nil
}

// ExecDelta runs delta on the provided unified diff and returns the highlighted output.
// Delta must be available in PATH (check with CheckDeltaAvailable first).
func ExecDelta(ctx context.Context, diff string) (string, error) {
	cmd := exec.CommandContext(ctx, "delta", "--paging=never", "--color-only")
	cmd.Stdin = bytes.NewBufferString(diff)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("exec delta: %w", err)
	}

	return stdout.String(), nil
}
