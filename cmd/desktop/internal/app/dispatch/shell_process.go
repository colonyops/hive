package dispatch

import (
	"context"
	"os/exec"
	"time"
)

func runShellProcess(ctx context.Context, command *exec.Cmd) error {
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
