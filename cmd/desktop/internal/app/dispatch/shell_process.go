package dispatch

import (
	"context"
	"os/exec"
	"time"
)

// RunShellProcess runs command in its own process group and stops the whole
// group when ctx ends, so a shell's children do not outlive a timeout.
func RunShellProcess(ctx context.Context, command *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	prepareShellProcess(command)
	command.WaitDelay = shellKillGrace
	if err := command.Start(); err != nil {
		return err
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	select {
	case err := <-wait:
		return err
	case <-ctx.Done():
	}

	_ = stopShellProcess(command, false)
	timer := time.NewTimer(shellKillGrace)
	defer timer.Stop()
	select {
	case <-wait:
		return ctx.Err()
	case <-timer.C:
		_ = stopShellProcess(command, true)
		<-wait
		return ctx.Err()
	}
}
