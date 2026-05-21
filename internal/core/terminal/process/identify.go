// Package process identifies AI agents from tmux pane process trees.
package process

import (
	"path/filepath"
	"strings"
)

const (
	toolAider     = "aider"
	toolClaude    = "claude"
	toolCodex     = "codex"
	toolCursor    = "cursor"
	toolGemini    = "gemini"
	toolOpencode  = "opencode"
	toolPi        = "pi"
	toolShell     = "shell"
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
func IdentifyWith(panePID int, r ProcessReader) (*AgentProcess, error) {
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

	foreground := readProcess(tpgid, r)
	if foreground.Tool != toolShell {
		return foreground, nil
	}

	if agent := findAgentChild(tpgid, r, 2); agent != nil {
		return agent, nil
	}

	return foreground, nil
}

func findAgentChild(rootPID int, r ProcessReader, maxDepth int) *AgentProcess {
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
			proc := readProcess(childPID, r)
			if proc.Tool != toolShell {
				return proc
			}
			queue = append(queue, queueItem{pid: childPID, depth: item.depth + 1})
		}
	}
	return nil
}

func readProcess(pid int, r ProcessReader) *AgentProcess {
	comm := r.Comm(pid)
	argv, _ := r.Cmdline(pid)
	env := r.Environ(pid)

	proc := &AgentProcess{PID: pid, Comm: comm, Argv: argv, Env: env, Tool: toolShell}
	if looksLikeClaude(env, argv) {
		proc.Tool = toolClaude
		return proc
	}
	if tool := toolFromArgv(argv); tool != "" {
		proc.Tool = tool
		return proc
	}
	if tool := toolFromBasename(comm); tool != "" {
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
func toolFromArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	base0 := strings.ToLower(filepath.Base(argv[0]))
	if tool := toolFromBasename(base0); tool != "" {
		return tool
	}

	switch base0 {
	case "node", "npx", "npm", "pnpm", "yarn", "bun", "uvx":
		return toolFromExecutableArgs(argv[1:])
	case "python", "python2", "python3":
		return toolFromPythonArgs(argv[1:])
	case "mise":
		return toolFromMiseArgs(argv[1:])
	case "env":
		return toolFromEnvArgs(argv[1:])
	default:
		return ""
	}
}

func toolFromExecutableArgs(args []string) string {
	for _, arg := range args {
		if shouldSkipExecutableArg(arg) {
			continue
		}
		if tool := toolFromBasename(filepath.Base(arg)); tool != "" {
			return tool
		}
	}
	return ""
}

func toolFromPythonArgs(args []string) string {
	for i, arg := range args {
		if arg == "-m" && i+1 < len(args) {
			return toolFromBasename(filepath.Base(args[i+1]))
		}
	}
	return ""
}

func toolFromMiseArgs(args []string) string {
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			return toolFromBasename(filepath.Base(args[i+1]))
		}
	}
	return toolFromExecutableArgs(args)
}

func toolFromEnvArgs(args []string) string {
	for _, arg := range args {
		if strings.Contains(arg, "=") || strings.HasPrefix(arg, "-") {
			continue
		}
		return toolFromBasename(filepath.Base(arg))
	}
	return ""
}

func shouldSkipExecutableArg(arg string) bool {
	return arg == "" || arg == "--" || strings.HasPrefix(arg, "-")
}

// toolFromBasename maps a process basename to an agent name.
func toolFromBasename(comm string) string {
	lower := strings.ToLower(comm)
	switch {
	case strings.Contains(lower, toolClaude):
		return toolClaude
	case strings.Contains(lower, toolGemini):
		return toolGemini
	case lower == toolAider:
		return toolAider
	case strings.Contains(lower, toolCodex):
		return toolCodex
	case strings.Contains(lower, toolCursor):
		return toolCursor
	case strings.Contains(lower, toolOpencode):
		return toolOpencode
	case lower == toolPi:
		return toolPi
	case strings.Contains(lower, "cline"):
		return "cline"
	default:
		return ""
	}
}
