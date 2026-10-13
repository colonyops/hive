//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package fswatch

import "syscall"

const (
	maxDescriptors      = 8192
	reservedDescriptors = 256
)

// descriptorBudget is how many descriptors the watches may hold. kqueue
// opens one per watched file, not per directory, so a large tree could
// otherwise use every descriptor the process has.
func descriptorBudget() (int, error) {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return maxDescriptors, err
	}
	if limit.Cur <= reservedDescriptors {
		return 1, nil
	}
	return int(min(limit.Cur-reservedDescriptors, maxDescriptors)), nil
}
