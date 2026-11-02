//go:build darwin

package process

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/sys/unix"
)

func tpgidFromPID(pid int) (int, error) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, err
	}
	return int(info.Eproc.Tpgid), nil
}

func commForPID(pid int) string {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ""
	}
	var name []byte
	for _, c := range info.Proc.P_comm {
		if c == 0 {
			break
		}
		name = append(name, c)
	}
	return string(name)
}

func cmdlineForPID(pid int) ([]string, error) {
	data, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(data) < 4 {
		return nil, nil
	}
	return parseKernProcargs2(data), nil
}

func childrenForPID(pid int) ([]int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	var children []int
	for _, proc := range procs {
		if int(proc.Eproc.Ppid) == pid {
			children = append(children, int(proc.Proc.P_pid))
		}
	}
	return children, nil
}

// parseKernProcargs2 returns argv from the KERN_PROCARGS2 sysctl output.
// Layout: [argc int32][exec_path\0][padding NULs][argv[0]\0 ... argv[n]\0][env\0...]
func parseKernProcargs2(data []byte) []string {
	if len(data) < 4 {
		return nil
	}
	argc := int(binary.LittleEndian.Uint32(data[:4]))
	rest := data[4:]

	// Skip exec_path (first NUL-terminated string after argc)
	if idx := bytes.IndexByte(rest, 0); idx >= 0 {
		rest = rest[idx+1:]
	} else {
		return nil
	}

	// Skip padding NULs
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}

	argv := make([]string, 0, argc)
	for i := 0; i < argc && len(rest) > 0; i++ {
		idx := bytes.IndexByte(rest, 0)
		if idx < 0 {
			argv = append(argv, string(rest))
			break
		}
		argv = append(argv, string(rest[:idx]))
		rest = rest[idx+1:]
	}
	return argv
}
