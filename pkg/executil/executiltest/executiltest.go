// Package executiltest provides a scripted executil.Executor test double.
package executiltest

import (
	"context"
	"io"
	"sync"

	"github.com/colonyops/hive/pkg/executil"
)

// Call records one executor invocation.
type Call struct {
	Cmd  string
	Dir  string
	Args []string
}

// Response scripts the result of one invocation, consumed in call order.
type Response struct {
	Out    []byte
	Stderr []byte
	Err    error
}

// Exec is a scripted executil.Executor: every invocation records a Call
// and consumes the next Response (zero-value results once the script runs
// out, so an unscripted Exec is a silent no-op stub). It also implements
// the optional RunOutput seam for separated stdout/stderr. Safe for
// concurrent use.
type Exec struct {
	mu        sync.Mutex
	calls     []Call
	Responses []Response
}

var _ executil.Executor = (*Exec)(nil)

// Calls returns the recorded invocations in call order.
func (e *Exec) Calls() []Call {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Call(nil), e.calls...)
}

func (e *Exec) record(cmd, dir string, args []string) Response {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, Call{Cmd: cmd, Dir: dir, Args: args})
	if idx := len(e.calls) - 1; idx < len(e.Responses) {
		return e.Responses[idx]
	}
	return Response{}
}

// Run returns the scripted response's combined output.
func (e *Exec) Run(_ context.Context, cmd string, args ...string) ([]byte, error) {
	resp := e.record(cmd, "", args)
	return append(append([]byte{}, resp.Out...), resp.Stderr...), resp.Err
}

// RunOutput returns the scripted response's stdout and stderr separately.
func (e *Exec) RunOutput(_ context.Context, cmd string, args ...string) ([]byte, []byte, error) {
	resp := e.record(cmd, "", args)
	return resp.Out, resp.Stderr, resp.Err
}

// RunOutputDir returns the scripted response's stdout and stderr separately
// and records the directory.
func (e *Exec) RunOutputDir(_ context.Context, dir, cmd string, args ...string) ([]byte, []byte, error) {
	resp := e.record(cmd, dir, args)
	return resp.Out, resp.Stderr, resp.Err
}

// RunDir behaves like Run and records the directory.
func (e *Exec) RunDir(_ context.Context, dir, cmd string, args ...string) ([]byte, error) {
	resp := e.record(cmd, dir, args)
	return append(append([]byte{}, resp.Out...), resp.Stderr...), resp.Err
}

// RunStream writes the scripted response to the provided writers.
func (e *Exec) RunStream(_ context.Context, stdout, stderr io.Writer, cmd string, args ...string) error {
	return e.stream(stdout, stderr, cmd, "", args)
}

// RunDirStream behaves like RunStream and records the directory.
func (e *Exec) RunDirStream(_ context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	return e.stream(stdout, stderr, cmd, dir, args)
}

func (e *Exec) stream(stdout, stderr io.Writer, cmd, dir string, args []string) error {
	resp := e.record(cmd, dir, args)
	if len(resp.Out) > 0 && stdout != nil {
		_, _ = stdout.Write(resp.Out)
	}
	if len(resp.Stderr) > 0 && stderr != nil {
		_, _ = stderr.Write(resp.Stderr)
	}
	return resp.Err
}
