package agentws

import (
	"path"
	"sort"
	"strings"
)

// Preset is a starter command template offered by the workspace editor. It is
// a shortcut, never a constraint: picking one fills the command field with
// text the user then owns and can edit freely
// (ADR the-workspace-command-is-a-template).
type Preset struct {
	// ID is stable across builds so the editor can key a row on it.
	ID string `json:"id"`
	// Agent is Command's own label (AgentFor), so the picker can mark the row.
	Agent string `json:"agent"`
	// Label names the row: a posture for a shipped preset, the profile key for
	// a hive-seeded one.
	Label string `json:"label"`
	// Command is the template written into the manifest verbatim.
	Command string `json:"command"`
	// Danger marks a preset carrying a known permission bypass, so the editor
	// can warn before it is chosen rather than only after.
	Danger bool `json:"danger"`
	// Source is "builtin" for a preset this build ships, "hive" for one seeded
	// from an agents: profile in hive's own config.
	Source string `json:"source"`
}

const (
	// PresetSourceBuiltin marks a preset shipped by this build.
	PresetSourceBuiltin = "builtin"
	// PresetSourceHive marks a preset seeded from hive's agents: profiles.
	PresetSourceHive = "hive"
)

// PromptTail is how a preset takes a scheduled chat's prompt: quoted as one
// trailing word behind the end-of-options marker, because a prompt is prose
// the user wrote and a markdown list ("- Summarize the week") would otherwise
// reach the CLI as an unknown option. It is exported so a refusal can tell the
// user exactly what a hand-written command is missing.
const PromptTail = ` {{ if .Prompt }}-- {{ .Prompt | shq }}{{ end }}`

// claudeTail is the wiring every claude preset shares: the generated
// .mcp.json (with --strict-mcp-config, so the tool set is exactly what the
// workspace declares), the session id, pinned as a resume or a fresh launch,
// and the prompt. It is duplicated into each preset's text rather than
// assembled at launch, because the whole point is that the user can see and
// edit it.
const claudeTail = ` --strict-mcp-config --mcp-config {{ .MCPConfig | shq }}` +
	` {{ if .Resume }}--resume{{ else }}--session-id{{ end }} {{ .SessionID }}` + PromptTail

// builtinPresets are the shortcuts for the CLIs this build knows the flags
// for. Codex has no launch-time MCP flag and no resume-by-id form, so its
// presets are the bare invocation plus the prompt: it discovers
// .codex/config.toml from the working directory, and a reopened codex session
// reattaches to its live tmux session rather than resuming a conversation.
var builtinPresets = []Preset{
	{
		ID: "claude-ask", Agent: "claude", Label: "Ask",
		Command: "claude" + claudeTail,
		Source:  PresetSourceBuiltin,
	},
	{
		ID: "claude-auto", Agent: "claude", Label: "Auto",
		Command: "claude --permission-mode acceptEdits" + claudeTail,
		Source:  PresetSourceBuiltin,
	},
	{
		ID: "claude-full", Agent: "claude", Label: "Full",
		Command: "claude --dangerously-skip-permissions" + claudeTail,
		Source:  PresetSourceBuiltin,
	},
	{
		ID: "codex-ask", Agent: "codex", Label: "Ask",
		Command: "codex" + PromptTail,
		Source:  PresetSourceBuiltin,
	},
	{
		ID: "codex-auto", Agent: "codex", Label: "Auto",
		Command: "codex --ask-for-approval on-request --sandbox workspace-write" + PromptTail,
		Source:  PresetSourceBuiltin,
	},
	{
		ID: "codex-full", Agent: "codex", Label: "Full",
		Command: "codex --dangerously-bypass-approvals-and-sandbox" + PromptTail,
		Source:  PresetSourceBuiltin,
	},
}

// BuiltinPresets returns the shipped presets, each with Danger derived from
// its own command rather than declared beside it — one source of truth, and
// the same derivation an edited command gets.
func BuiltinPresets() []Preset {
	out := make([]Preset, len(builtinPresets))
	for i, p := range builtinPresets {
		p.Danger = CommandIsDangerous(p.Command)
		out[i] = p
	}
	return out
}

// PresetCommand returns the shipped preset's command, or "" for an unknown id.
// The seeded hive workspace names one by id so the two cannot fall out of step.
func PresetCommand(id string) string {
	for _, p := range builtinPresets {
		if p.ID == id {
			return p.Command
		}
	}
	return ""
}

// AgentFor derives a command's agent label: the basename of its first word,
// lowercased. It replaces the manifest's stored agent key
// (ADR the-workspace-command-is-a-template).
//
// A wrapper or an env prefix (`env FOO=1 claude`) derives to a name this build
// knows nothing about, which falls back to the generic activity patterns and
// to a resume probe that assumes a conversation exists. Both degrade toward
// launching, never toward refusing.
func AgentFor(command string) string {
	word, _, _ := strings.Cut(strings.TrimSpace(command), " ")
	if word == "" {
		return ""
	}
	return strings.ToLower(path.Base(word))
}

// SortPresets orders presets for display: shipped before seeded, then by
// agent, then by the order this file declares them. A map-ranged caller
// (hive's profiles) has no order of its own, so this is what keeps the
// editor's list stable between refreshes.
func SortPresets(presets []Preset) {
	rank := make(map[string]int, len(builtinPresets))
	for i, p := range builtinPresets {
		rank[p.ID] = i
	}
	sort.SliceStable(presets, func(i, j int) bool {
		a, b := presets[i], presets[j]
		if (a.Source == PresetSourceBuiltin) != (b.Source == PresetSourceBuiltin) {
			return a.Source == PresetSourceBuiltin
		}
		if a.Agent != b.Agent {
			return a.Agent < b.Agent
		}
		return rank[a.ID] < rank[b.ID]
	})
}

// WiringTailFor returns the launch-time wiring a CLI needs appended to a bare
// invocation: the generated MCP config and the session id, for the agents this
// build knows the flags for. Empty for anything else.
//
// It exists for hive-seeded presets. A profile is a command word plus flags
// and carries no wiring of its own, so a profile running `claude` under
// another name (a model-pinned "fable" profile) would otherwise seed a preset
// that launches with no MCP servers and a session Hive cannot address. The
// match is on the command word, which is the only thing that decides which
// flags the binary accepts.
func WiringTailFor(command string) string {
	if AgentFor(command) == "claude" {
		return claudeTail
	}
	return ""
}
