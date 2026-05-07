// Package process identifies AI agents from tmux pane process trees.
package process

import (
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/core/terminal"
)

// AgentProcess describes the foreground process in a tmux pane.
type AgentProcess struct {
	// Tool is the matched agent name (e.g. "claude", "codex"), or "" if no
	// known agent was identified. Callers should treat "" as "unknown" and
	// fall back to other detection methods (e.g. content-based).
	Tool string
	PID  int               // foreground process group leader PID
	Comm string            // short process name
	Argv []string          // full argument list (may be nil)
	Env  map[string]string // environment (nil if unavailable, e.g. macOS hardened runtime)
}

// Process readers, indirected through package vars so tests can substitute
// platform-specific syscalls with fixtures.
var (
	readTpgid = tpgidFromPID
	readComm  = commForPID
	readArgv  = cmdlineForPID
	readEnv   = environForPID
)

// Identify inspects the foreground process for the pane rooted at panePID and
// returns an AgentProcess describing it. Tool is "" when no known agent is
// matched — the caller is expected to treat that as unknown and fall back to
// content-based detection. Returns nil if panePID is zero or negative.
func Identify(panePID int) (*AgentProcess, error) {
	if panePID <= 0 {
		return nil, nil
	}

	tpgid, err := readTpgid(panePID)
	if err != nil || tpgid <= 0 {
		tpgid = panePID
	}

	comm := readComm(tpgid)
	argv, _ := readArgv(tpgid)
	env := readEnv(tpgid)

	proc := &AgentProcess{PID: tpgid, Comm: comm, Argv: argv, Env: env}

	switch {
	case looksLikeClaude(env, argv):
		proc.Tool = "claude"
	default:
		if t := toolFromArgv(argv); t != "" {
			proc.Tool = t
		} else if t := toolFromBasename(comm); t != "" {
			proc.Tool = t
		}
	}

	return proc, nil
}

// looksLikeClaude returns true if env or argv indicate the Claude Code agent.
// CLAUDECODE=1 is set on the Claude Code process itself. On macOS with hardened
// runtime, env may be nil — fall back to argv.
func looksLikeClaude(env map[string]string, argv []string) bool {
	if env["CLAUDECODE"] == "1" {
		return true
	}
	if len(argv) > 0 && strings.Contains(strings.ToLower(filepath.Base(argv[0])), "claude") {
		return true
	}
	return false
}

// wrapperBinaries are interpreter/launcher commands whose argv[1] (when present)
// is the actual program being run.
var wrapperBinaries = map[string]bool{
	"npx": true, "node": true, "python": true, "python2": true, "python3": true,
}

// toolFromArgv identifies the agent tool from the full argv, handling wrapper
// invocations like "npx claude" or "python3 -m aider".
func toolFromArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	base0 := strings.ToLower(filepath.Base(argv[0]))
	if wrapperBinaries[base0] && len(argv) > 1 {
		return toolFromBasename(filepath.Base(argv[1]))
	}
	return toolFromBasename(filepath.Base(argv[0]))
}

// toolFromBasename maps a process basename to a known agent name. Returns ""
// if no agent matches.
func toolFromBasename(comm string) string {
	return terminal.MatchProcessBasename(comm)
}
