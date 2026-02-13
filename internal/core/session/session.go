// Package session defines session domain types and interfaces.
package session

import (
	"regexp"
	"strings"
	"time"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a name to a URL-safe slug.
// "My Session Name" -> "my-session-name"
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// State represents the lifecycle state of a session.
// ENUM(active, recycled, corrupted).
type State string

// Metadata keys for terminal integration.
const (
	MetaTmuxSession = "tmux_session" // tmux session name
	MetaTmuxPane    = "tmux_pane"    // tmux pane identifier
)

// Session represents an isolated git environment for an AI agent.
//
// Terminology:
//   - Session: The hive-managed git clone + terminal environment
//   - Agent: The AI tool (Claude, Aider, etc.) running within the session
//   - Tmux session: The terminal multiplexer session (if tmux integration enabled)
//
// Relationship: Hive Session → spawns → Tmux Session → runs → Agent
//
// Future: Hive will support multiple agents per session, enabling
// concurrent agents like a primary assistant and a test runner.
type Session struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	Path      string            `json:"path"`
	Remote    string            `json:"remote"`
	State     State             `json:"state"`
	Metadata  map[string]string `json:"metadata,omitempty"` // integration data (e.g., tmux session name)
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// InboxTopic returns the conventional inbox topic name for this session.
//
// Format: agent.<session-id>.inbox
//
// The "agent" prefix refers to the AI agent running in the session.
// When multi-agent support is added, named agents will use:
// agent.<session-id>.<agent-name>.inbox
//
// Example: agent.26kj0c.inbox
func (s *Session) InboxTopic() string {
	return "agent." + s.ID + ".inbox"
}

// CanRecycle returns true if the session can be marked for recycling.
func (s *Session) CanRecycle() bool {
	return s.State == StateActive
}

// MarkRecycled transitions the session to the recycled state.
func (s *Session) MarkRecycled(now time.Time) {
	s.State = StateRecycled
	s.UpdatedAt = now
}

// MarkCorrupted transitions the session to the corrupted state.
func (s *Session) MarkCorrupted(now time.Time) {
	s.State = StateCorrupted
	s.UpdatedAt = now
}

// GetMeta returns the value for the given metadata key, or empty string if not set.
func (s *Session) GetMeta(key string) string {
	if s.Metadata == nil {
		return ""
	}
	return s.Metadata[key]
}

// SetMeta sets a metadata key-value pair, initializing the map if needed.
func (s *Session) SetMeta(key, value string) {
	if s.Metadata == nil {
		s.Metadata = make(map[string]string)
	}
	s.Metadata[key] = value
}
