// Package process reads OS-level process information for tmux panes.
// It performs no classification; callers decide which processes represent agents.
package process

// ProcessInfo holds raw OS-level data for a single process.
type ProcessInfo struct {
	PID  int
	Comm string            // short binary name (from p_comm / /proc/N/comm)
	Argv []string          // full argument list (may be nil on macOS hardened runtime)
	Env  map[string]string // environment (nil if unavailable)
}

// CandidateProcesses is returned by Candidates. It contains the foreground
// process for a pane plus any children at depths 1 and 2, which are needed
// to detect wrapper invocations (e.g. node → claude, sh → aider).
// Classifying which, if any, of these processes is an agent is the caller's
// responsibility.
type CandidateProcesses struct {
	Foreground ProcessInfo
	Children   []ProcessInfo
}

// Candidates resolves the foreground process for panePID and collects child
// processes up to depth 2. It performs no classification.
func Candidates(panePID int, r ProcessReader) (*CandidateProcesses, error) {
	if panePID <= 0 {
		return nil, nil
	}
	if r == nil {
		r = OSReader{}
	}

	tpgid, err := r.TPGID(panePID)
	if err != nil || tpgid <= 0 {
		tpgid = panePID
	}

	result := &CandidateProcesses{
		Foreground: readProcessInfo(tpgid, r),
		Children:   walkChildren(tpgid, r, 2),
	}
	return result, nil
}

func walkChildren(rootPID int, r ProcessReader, maxDepth int) []ProcessInfo {
	type item struct {
		pid   int
		depth int
	}
	visited := map[int]bool{rootPID: true}
	queue := []item{{pid: rootPID, depth: 0}}
	var out []ProcessInfo

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr.depth >= maxDepth {
			continue
		}
		children, err := r.Children(curr.pid)
		if err != nil {
			continue
		}
		for _, childPID := range children {
			if childPID <= 0 || visited[childPID] {
				continue
			}
			visited[childPID] = true
			out = append(out, readProcessInfo(childPID, r))
			queue = append(queue, item{pid: childPID, depth: curr.depth + 1})
		}
	}
	return out
}

func readProcessInfo(pid int, r ProcessReader) ProcessInfo {
	argv, _ := r.Cmdline(pid)
	return ProcessInfo{
		PID:  pid,
		Comm: r.Comm(pid),
		Argv: argv,
		Env:  r.Environ(pid),
	}
}
