package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PortLister returns listening TCP ports keyed by owning PID.
type PortLister interface {
	ListListeningPorts(ctx context.Context, pids []int) (map[int][]int, error)
}

type LsofPortLister struct{}

func (LsofPortLister) ListListeningPorts(ctx context.Context, pids []int) (map[int][]int, error) {
	pids = uniquePositiveInts(pids)
	if len(pids) == 0 {
		return nil, nil
	}
	pidArgs := make([]string, len(pids))
	for i, pid := range pids {
		pidArgs[i] = strconv.Itoa(pid)
	}
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strings.Join(pidArgs, ",")).Output()
	if err != nil {
		return nil, fmt.Errorf("lsof listening ports: %w", err)
	}
	return parseLsofListeningPorts(string(out)), nil
}

var lsofPortPattern = regexp.MustCompile(`:(\d+)\s+\(LISTEN\)$`)

func parseLsofListeningPorts(output string) map[int][]int {
	portsByPID := make(map[int][]int)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "COMMAND" {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil || pid <= 0 {
			continue
		}
		matches := lsofPortPattern.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}
		port, err := strconv.Atoi(matches[1])
		if err != nil || port <= 0 {
			continue
		}
		portsByPID[pid] = append(portsByPID[pid], port)
	}
	for pid, ports := range portsByPID {
		portsByPID[pid] = uniqueSortedPorts(ports)
	}
	return portsByPID
}

func uniquePositiveInts(values []int) []int {
	seen := make(map[int]bool, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func uniqueSortedPorts(values []int) []int {
	return uniquePositiveInts(values)
}
