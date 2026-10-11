//go:build windows

package logutils

// Windows cannot rename a file another process holds open, so a rotation
// there fails and the log keeps growing; the lock has nothing to guard.
func lockFile(string) (func(), error) {
	return func() {}, nil
}
