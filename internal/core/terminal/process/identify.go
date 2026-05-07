// Package process identifies AI agents from tmux pane process trees.
package process

import (
	"path/filepath"
	"strings"
)

// AgentProcess describes the foreground process in a tmux pane.
type AgentProcess struct {
	Tool string            // detected agent ("claude", "gemini", etc.) or "shell"
	PID  int               // foreground process group leader PID
	Comm string            // short process name
	Argv []string          // full argument list (may be nil)
	Env  map[string]string // environment (nil if unavailable, e.g. macOS hardened runtime)
}

// Identify walks the process tree rooted at panePID and returns the
// foreground agent, or nil if panePID is zero/negative.
// Errors from process reads are non-fatal: the function degrades to
// argv/comm matching when env or tpgid are unavailable.
func Identify(panePID int) (*AgentProcess, error) {
	if panePID <= 0 {
		return nil, nil
	}

	tpgid, err := tpgidFromPID(panePID)
	if err != nil || tpgid <= 0 {
		tpgid = panePID
	}

	comm := commForPID(tpgid)
	argv, _ := cmdlineForPID(tpgid)
	env := environForPID(tpgid)

	proc := &AgentProcess{PID: tpgid, Comm: comm, Argv: argv, Env: env}

	switch {
	case looksLikeClaude(env, argv):
		proc.Tool = "claude"
	case toolFromArgv(argv) != "":
		proc.Tool = toolFromArgv(argv)
	case toolFromBasename(comm) != "":
		proc.Tool = toolFromBasename(comm)
	default:
		proc.Tool = "shell"
	}

	return proc, nil
}

// looksLikeClaude returns true if env or argv indicate the Claude Code agent.
// CLAUDECODE=1 is set on the Claude Code process itself (confirmed by SDK).
// On macOS with hardened runtime, env may be nil — fall back to argv/comm.
func looksLikeClaude(env map[string]string, argv []string) bool {
	if env["CLAUDECODE"] == "1" {
		return true
	}
	if len(argv) > 0 && strings.Contains(strings.ToLower(filepath.Base(argv[0])), "claude") {
		return true
	}
	return false
}

// toolFromArgv identifies the agent tool from the full argv, handling wrapper
// invocations like "npx claude" or "python3 -m aider".
func toolFromArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	base0 := strings.ToLower(filepath.Base(argv[0]))
	wrappers := map[string]bool{"npx": true, "node": true, "python3": true, "python": true, "python2": true}
	if wrappers[base0] && len(argv) > 1 {
		return toolFromBasename(filepath.Base(argv[1]))
	}
	return toolFromBasename(filepath.Base(argv[0]))
}

// toolFromBasename maps a process basename to an agent name.
func toolFromBasename(comm string) string {
	lower := strings.ToLower(comm)
	switch {
	case strings.Contains(lower, "claude"):
		return "claude"
	case strings.Contains(lower, "gemini"):
		return "gemini"
	case lower == "aider":
		return "aider"
	case strings.Contains(lower, "codex"):
		return "codex"
	case strings.Contains(lower, "cursor"):
		return "cursor"
	case strings.Contains(lower, "opencode"):
		return "opencode"
	case strings.Contains(lower, "cline"):
		return "cline"
	default:
		return ""
	}
}

