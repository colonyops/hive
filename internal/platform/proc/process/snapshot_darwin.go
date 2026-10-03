//go:build darwin

package process

import "golang.org/x/sys/unix"

// snapshotChildren reads the Darwin kernel process table once and returns a
// ppid → child PID slice map. A single SysctlKinfoProcSlice call replaces the
// per-pane calls that would otherwise occur during process tree traversal.
func snapshotChildren() map[int][]int {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil
	}
	m := make(map[int][]int, len(procs))
	for _, p := range procs {
		ppid := int(p.Eproc.Ppid)
		pid := int(p.Proc.P_pid)
		if pid > 0 {
			m[ppid] = append(m[ppid], pid)
		}
	}
	return m
}
