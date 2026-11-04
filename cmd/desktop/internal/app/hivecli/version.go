package hivecli

import (
	"context"
	osexec "os/exec"
	"strings"
	"time"
)

// RunVersion returns the output of `<path> --version`.
func RunVersion(ctx context.Context, path string, env []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, path, "--version")
	cmd.Env = env
	out, err := cmd.Output()
	return string(out), err
}

// ParseVersion reads the first token after "version " in `hive --version`
// output ("hive version 1.2.3 (abc1234) 2026-01-01").
func ParseVersion(out string) (string, bool) {
	_, rest, found := strings.Cut(out, "version ")
	if !found {
		return "", false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}
