//go:build !windows

package daemon

import "syscall"

// setUmask changes the process umask, which is process-wide: the daemon calls
// it only during startup, before it serves anything.
func setUmask(mask int) (restore func()) {
	old := syscall.Umask(mask)
	return func() { syscall.Umask(old) }
}
