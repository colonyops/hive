//go:build darwin

package process

import (
	"bytes"
	"encoding/binary"
	"strings"

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
	argv, _ := kernProcArgs2(pid)
	return argv, nil
}

func environForPID(pid int) map[string]string {
	_, env := kernProcArgs2(pid)
	return env
}

// kernProcArgs2 fetches argv and env for a process via kern.procargs2.
// On macOS with hardened runtime, env may be empty (zeroed by kernel) — in
// that case env is returned as nil to signal unavailability.
func kernProcArgs2(pid int) (argv []string, env map[string]string) {
	data, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(data) < 4 {
		return nil, nil
	}
	return parseKernProcargs2(data)
}

// maxArgcPrealloc bounds the slice prealloc derived from the kernel-supplied
// argc to avoid a giant allocation if the sysctl buffer is corrupted.
// Real argc is small (usually <100); the bound is generous.
const maxArgcPrealloc = 4096

// parseKernProcargs2 parses the KERN_PROCARGS2 sysctl output.
// Layout: [argc int32][exec_path\0][padding NULs][argv[0]\0 ... argv[n]\0][env\0...]
func parseKernProcargs2(data []byte) (argv []string, env map[string]string) {
	if len(data) < 4 {
		return nil, nil
	}
	argc := int(binary.LittleEndian.Uint32(data[:4]))
	rest := data[4:]

	// Skip exec_path (first NUL-terminated string after argc)
	if idx := bytes.IndexByte(rest, 0); idx >= 0 {
		rest = rest[idx+1:]
	} else {
		return nil, nil
	}

	// Skip padding NULs
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}

	// Parse argv
	prealloc := argc
	if prealloc < 0 || prealloc > maxArgcPrealloc {
		prealloc = 0
	}
	argv = make([]string, 0, prealloc)
	for i := 0; i < argc && len(rest) > 0; i++ {
		idx := bytes.IndexByte(rest, 0)
		if idx < 0 {
			argv = append(argv, string(rest))
			rest = nil
			break
		}
		argv = append(argv, string(rest[:idx]))
		rest = rest[idx+1:]
	}

	// Parse env (NUL-terminated key=value pairs until empty entry or end)
	env = make(map[string]string)
	for len(rest) > 0 {
		idx := bytes.IndexByte(rest, 0)
		var entry string
		if idx < 0 {
			entry = string(rest)
			rest = nil
		} else {
			entry = string(rest[:idx])
			rest = rest[idx+1:]
		}
		if entry == "" {
			break
		}
		k, v, _ := strings.Cut(entry, "=")
		if k != "" {
			env[k] = v
		}
	}
	if len(env) == 0 {
		env = nil // unavailable (hardened runtime may zero env)
	}
	return argv, env
}
