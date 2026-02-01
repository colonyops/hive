package initcmd

import "os/exec"

// TmuxAvailable checks if tmux is installed and accessible.
func TmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}
