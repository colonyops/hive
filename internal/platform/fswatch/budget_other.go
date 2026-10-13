//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd)

package fswatch

// descriptorBudget is no cap: inotify and ReadDirectoryChangesW hold one
// handle per directory, not per file.
func descriptorBudget() (int, error) { return 0, nil }
