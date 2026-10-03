//go:build windows

package execenv

import "os/exec"

func isolateShellProbe(_ *exec.Cmd) {}
