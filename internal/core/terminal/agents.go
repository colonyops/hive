package terminal

import (
	"path/filepath"
	"strings"
)

// AgentDef declares everything the detector needs to recognize one AI agent.
// Process detection inspects the foreground binary's basename; content detection
// scans pane output for keywords. An agent may participate in either or both.
type AgentDef struct {
	Name string

	// ProcMatch is a basename substring (case-insensitive) for process-tree
	// detection. Empty disables process matching for this agent.
	ProcMatch string
	// ProcExact requires basename == ProcMatch instead of substring match.
	// Used when the basename is too generic for substring matching (e.g. "aider").
	ProcExact bool

	// ContentKeywords trigger content-based detection when present in pane output.
	ContentKeywords []string
}

// KnownAgents is the canonical list of agents the detector knows about.
// All agent matching (process tree, content patterns) derives from this list,
// so adding an agent only requires editing here. Order matters: first match wins.
var KnownAgents = []AgentDef{
	{Name: "claude", ProcMatch: "claude", ContentKeywords: []string{"claude", "anthropic", "ctrl+c to interrupt"}},
	{Name: "cursor", ProcMatch: "cursor", ContentKeywords: []string{"cursor"}},
	{Name: "crush", ProcMatch: "crush", ContentKeywords: []string{"crush"}},
	{Name: "gemini", ProcMatch: "gemini", ContentKeywords: []string{"gemini", "google ai"}},
	{Name: "opencode", ProcMatch: "opencode", ContentKeywords: []string{"opencode", "open code"}},
	{Name: "codex", ProcMatch: "codex", ContentKeywords: []string{"codex", "openai"}},
	{Name: "aider", ProcMatch: "aider", ProcExact: true},
	{Name: "cline", ProcMatch: "cline"},
	{Name: "agent", ContentKeywords: []string{"agent"}},
}

// MatchProcessBasename returns the agent name whose ProcMatch matches comm
// (case-insensitive), or "" if no agent matches.
func MatchProcessBasename(comm string) string {
	if comm == "" {
		return ""
	}
	lower := strings.ToLower(filepath.Base(comm))
	for _, a := range KnownAgents {
		if a.ProcMatch == "" {
			continue
		}
		if a.ProcExact {
			if lower == a.ProcMatch {
				return a.Name
			}
			continue
		}
		if strings.Contains(lower, a.ProcMatch) {
			return a.Name
		}
	}
	return ""
}
