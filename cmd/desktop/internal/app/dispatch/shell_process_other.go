//go:build !unix

package dispatch

import "os/exec"

func prepareShellProcess(_ *exec.Cmd) {}

func stopShellProcess(command *exec.Cmd, _ bool) error {
	return command.Process.Kill()
}
