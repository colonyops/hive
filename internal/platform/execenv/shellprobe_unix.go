//go:build darwin || linux

package execenv

import (
	"os/exec"
	"syscall"
)

func isolateShellProbe(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
