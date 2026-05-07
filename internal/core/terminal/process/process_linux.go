//go:build linux

package process

import (
	"fmt"
	"os"
	"strings"
)

func tpgidFromPID(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	return parseTpgid(string(data))
}

// parseTpgid parses the tpgid field from /proc/N/stat content.
// The comm field (field 2) is in parentheses and may contain spaces,
// so we scan from the last ')' to locate subsequent numeric fields.
// Field indices after ')': state(0) ppid(1) pgrp(2) session(3) tty_nr(4) tpgid(5)
func parseTpgid(stat string) (int, error) {
	idx := strings.LastIndex(stat, ")")
	if idx < 0 {
		return 0, fmt.Errorf("malformed stat: no closing paren")
	}
	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 6 {
		return 0, fmt.Errorf("malformed stat: too few fields after comm")
	}
	var tpgid int
	if _, err := fmt.Sscanf(fields[5], "%d", &tpgid); err != nil {
		return 0, fmt.Errorf("parse tpgid: %w", err)
	}
	return tpgid, nil
}

func commForPID(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cmdlineForPID(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(data) == 0 {
		return nil, err
	}
	var parts []string
	for part := range strings.SplitSeq(strings.TrimRight(string(data), "\x00"), "\x00") {
		parts = append(parts, part)
	}
	return parts, nil
}

func environForPID(pid int) map[string]string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil || len(data) == 0 {
		return nil
	}
	env := make(map[string]string)
	for entry := range strings.SplitSeq(strings.TrimRight(string(data), "\x00"), "\x00") {
		k, v, _ := strings.Cut(entry, "=")
		if k != "" {
			env[k] = v
		}
	}
	if len(env) == 0 {
		return nil
	}
	return env
}
