//go:build linux

package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func childrenForPID(pid int) ([]int, error) {
	children, err := childrenFromTaskFiles(pid)
	if err == nil {
		return children, nil
	}
	return childrenByScanningProc(pid)
}

func childrenFromTaskFiles(pid int) ([]int, error) {
	matches, err := filepath.Glob(fmt.Sprintf("/proc/%d/task/*/children", pid))
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no task children files for pid %d", pid)
	}

	seen := make(map[int]bool)
	var children []int
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		for _, field := range strings.Fields(string(data)) {
			childPID, err := strconv.Atoi(field)
			if err != nil || childPID <= 0 || seen[childPID] {
				continue
			}
			seen[childPID] = true
			children = append(children, childPID)
		}
	}
	return children, nil
}

func childrenByScanningProc(pid int) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var children []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		childPID, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", childPID))
		if err != nil {
			continue
		}
		ppid, err := parsePPID(string(data))
		if err != nil {
			continue
		}
		if ppid == pid {
			children = append(children, childPID)
		}
	}
	return children, nil
}

func parsePPID(stat string) (int, error) {
	idx := strings.LastIndex(stat, ")")
	if idx < 0 {
		return 0, fmt.Errorf("malformed stat: no closing paren")
	}
	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 2 {
		return 0, fmt.Errorf("malformed stat: too few fields after comm")
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("parse ppid: %w", err)
	}
	return ppid, nil
}
