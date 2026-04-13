package terminal

import "time"

// HookStatus is stored in SQLite KV for each Claude Code hook event.
// Key: "hook.status:<session-id>:<window-index>" (window-index "0" for single-window).
//
// Staleness is determined application-side via WrittenAt + freshness window,
// not by KV TTL. The 24h TTL on the KV entry is for orphan cleanup only.
type HookStatus struct {
	Status      Status    `json:"status"`
	Event       string    `json:"event"`
	WindowIndex string    `json:"window_index"`
	WrittenAt   time.Time `json:"written_at"`
}

// ToolClaude is the tool identifier for Claude Code hook-sourced status entries.
const ToolClaude = "claude"

// DefaultHookFreshnessWindow is the duration after which a hook status entry is
// considered stale and tmux fallback is used instead.
const DefaultHookFreshnessWindow = 2 * time.Minute
