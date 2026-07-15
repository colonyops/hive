package rpc

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const (
	maxOutputBytes = 8 << 20 // 8 MiB per output stream
	execWaitDelay  = time.Second
)

// ProcessRunner executes one source subprocess request. argv is passed directly
// to the operating system without a shell, env is the complete child
// environment, and stdin contains one newline-terminated JSON-RPC request.
type ProcessRunner interface {
	Run(ctx context.Context, argv, env []string, stdin []byte) (stdout, stderr []byte, err error)
}

// ExecProcessRunner runs source subprocesses with exec.CommandContext.
type ExecProcessRunner struct{}

// Run executes argv directly, writes stdin, and captures stdout and stderr in
// separate bounded buffers. Each buffer retains at most maxOutputBytes+1 bytes
// so the caller can distinguish an exact-limit response from overflow.
func (ExecProcessRunner) Run(ctx context.Context, argv, env []string, stdin []byte) (stdout, stderr []byte, err error) {
	if len(argv) == 0 {
		return nil, nil, fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.WaitDelay = execWaitDelay

	var stdoutBuffer, stderrBuffer limitedBuffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	runErr := cmd.Run()
	stdout = stdoutBuffer.Bytes()
	stderr = stderrBuffer.Bytes()
	if runErr != nil {
		return stdout, stderr, fmt.Errorf("execute %q: %w", argv[0], runErr)
	}
	return stdout, stderr, nil
}

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if remaining := maxOutputBytes + 1 - b.buf.Len(); remaining > 0 {
		_, _ = b.buf.Write(p[:min(len(p), remaining)])
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}
