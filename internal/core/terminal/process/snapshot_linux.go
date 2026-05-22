//go:build linux

package process

import (
	"fmt"
	"os"
	"strconv"
)

// snapshotChildren scans /proc once and returns a ppid → child PID slice map.
// This replaces per-pane /proc scans with a single pass during a refresh cycle.
func snapshotChildren() map[int][]int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	m := make(map[int][]int, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			continue
		}
		ppid, err := parsePPID(string(data))
		if err != nil || ppid <= 0 {
			continue
		}
		m[ppid] = append(m[ppid], pid)
	}
	return m
}
