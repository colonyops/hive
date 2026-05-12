//go:build windows

package timer

import "errors"

// ForkChild returns an error on Windows: hive timer relies on POSIX
// setsid semantics to detach the child from the controlling terminal,
// which Windows does not support natively.
func ForkChild(selfPath string, args []string, env []string) (int, error) {
	return 0, errors.New("timer: detached fork not supported on this platform")
}
