// Package process identifies AI agents from tmux pane process trees.
package process

import (
	"path/filepath"
	"strings"
)

const (
	// ToolShell is the sentinel tool name returned when the foreground process is
	// an interactive shell rather than a recognised agent.
	ToolShell = "shell"

	// envClaudeCode is injected by the Claude Code SDK (CLAUDECODE=1) and is
	// used as a reliable signal even when argv is obscured (macOS hardened runtime).
	envClaudeCode = "CLAUDECODE"
)

// AgentProcess describes the foreground process in a tmux pane.
type AgentProcess struct {
	Tool string            // detected agent ("claude", "gemini", etc.) or "shell"
	PID  int               // matched process PID (foreground process group leader or child agent)
	Comm string            // short process name
	Argv []string          // full argument list (may be nil)
	Env  map[string]string // environment (nil if unavailable, e.g. macOS hardened runtime)
}

// IdentifyWith walks the process tree using the provided reader.
// It inspects the foreground process first, then walks child processes
// to depth 2 to catch wrappers (e.g., node → claude, sh → claude).
// knownTools is the list of agent binary-name substrings to recognise
// (e.g. ["claude", "aider"]); it comes from the caller's config so no
// source-code change is required to support a new tool.
func IdentifyWith(panePID int, r ProcessReader, knownTools []string) (*AgentProcess, error) {
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

	foreground := readProcess(tpgid, r, knownTools)
	if foreground.Tool != ToolShell {
		return foreground, nil
	}

	if agent := findAgentChild(tpgid, r, 2, knownTools); agent != nil {
		return agent, nil
	}

	return foreground, nil
}

func findAgentChild(rootPID int, r ProcessReader, maxDepth int, knownTools []string) *AgentProcess {
	type queueItem struct {
		pid   int
		depth int
	}

	visited := map[int]bool{rootPID: true}
	queue := []queueItem{{pid: rootPID, depth: 0}}
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.depth >= maxDepth {
			continue
		}

		children, err := r.Children(item.pid)
		if err != nil {
			continue
		}
		for _, childPID := range children {
			if childPID <= 0 || visited[childPID] {
				continue
			}
			visited[childPID] = true
			proc := readProcess(childPID, r, knownTools)
			if proc.Tool != ToolShell {
				return proc
			}
			queue = append(queue, queueItem{pid: childPID, depth: item.depth + 1})
		}
	}
	return nil
}

func readProcess(pid int, r ProcessReader, knownTools []string) *AgentProcess {
	comm := r.Comm(pid)
	argv, _ := r.Cmdline(pid)
	env := r.Environ(pid)

	proc := &AgentProcess{PID: pid, Comm: comm, Argv: argv, Env: env, Tool: ToolShell}
	if looksLikeClaude(env, argv) {
		proc.Tool = "claude"
		return proc
	}
	if tool := toolFromArgv(argv, knownTools); tool != "" {
		proc.Tool = tool
		return proc
	}
	if tool := toolFromBasename(comm, knownTools); tool != "" {
		proc.Tool = tool
	}
	return proc
}

// looksLikeClaude returns true if env or argv indicate the Claude Code agent.
// CLAUDECODE=1 is set on the Claude Code process itself (confirmed by SDK).
// On macOS with hardened runtime, env may be nil — fall back to argv/comm.
func looksLikeClaude(env map[string]string, argv []string) bool {
	if env[envClaudeCode] == "1" {
		return true
	}
	if len(argv) > 0 && strings.Contains(strings.ToLower(filepath.Base(argv[0])), "claude") {
		return true
	}
	return false
}

// toolFromArgv identifies the agent tool from the full argv, handling wrapper
// invocations like "npx claude" or "python3 -m aider". Interactive shells are
// intentionally not parsed here; shell-launched agents are detected by child
// process walking so command strings like `bash -lc "echo claude"` do not match.
func toolFromArgv(argv []string, tools []string) string {
	if len(argv) == 0 {
		return ""
	}
	base0 := strings.ToLower(filepath.Base(argv[0]))
	if tool := toolFromBasename(base0, tools); tool != "" {
		return tool
	}

	switch base0 {
	case "node", "npx", "npm", "pnpm", "yarn", "bun", "uvx":
		return toolFromExecutableArgs(argv[1:], tools)
	case "python", "python2", "python3":
		return toolFromPythonArgs(argv[1:], tools)
	case "mise":
		return toolFromMiseArgs(argv[1:], tools)
	case "env":
		return toolFromEnvArgs(argv[1:], tools)
	default:
		return ""
	}
}

func toolFromExecutableArgs(args []string, tools []string) string {
	for _, arg := range args {
		if shouldSkipExecutableArg(arg) {
			continue
		}
		if tool := toolFromBasename(filepath.Base(arg), tools); tool != "" {
			return tool
		}
	}
	return ""
}

func toolFromPythonArgs(args []string, tools []string) string {
	for i, arg := range args {
		if arg == "-m" && i+1 < len(args) {
			return toolFromBasename(filepath.Base(args[i+1]), tools)
		}
	}
	return ""
}

func toolFromMiseArgs(args []string, tools []string) string {
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			return toolFromBasename(filepath.Base(args[i+1]), tools)
		}
	}
	return toolFromExecutableArgs(args, tools)
}

func toolFromEnvArgs(args []string, tools []string) string {
	for _, arg := range args {
		if strings.Contains(arg, "=") || strings.HasPrefix(arg, "-") {
			continue
		}
		return toolFromBasename(filepath.Base(arg), tools)
	}
	return ""
}

func shouldSkipExecutableArg(arg string) bool {
	return arg == "" || arg == "--" || strings.HasPrefix(arg, "-")
}

// toolFromBasename matches a process basename against the caller-provided list
// of known tool names. It accepts an exact match or a name that is a
// hyphen/underscore-prefixed variant (e.g. "claude-code" matches "claude").
func toolFromBasename(comm string, tools []string) string {
	lower := strings.ToLower(comm)
	for _, tool := range tools {
		if matchesToolName(lower, tool) {
			return tool
		}
	}
	return ""
}

// matchesToolName reports whether a process basename corresponds to tool.
// Exact equality and hyphen/underscore-delimited variants are accepted so
// that e.g. "claude-code" matches "claude" while "pip" does not match "pi".
func matchesToolName(basename, tool string) bool {
	return basename == tool ||
		strings.HasPrefix(basename, tool+"-") ||
		strings.HasPrefix(basename, tool+"_")
}
