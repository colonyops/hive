// Package session defines session domain types and interfaces.
package session

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// validName matches the allowed session name character set.
// Blocks characters that are meaningless or actively harmful in session names
// while allowing the full range developers commonly use in branch/ticket names.
// Disallowed: ~ ^ * ? [ \ @ and control characters.
var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.:/\-]*$`)

// ValidateName returns an error if name contains characters outside the
// allowed set. Slugify maps all non-alphanumeric characters to hyphens, so
// the derived slug is always safe for tmux session names and git branch names
// regardless of what permitted characters appear in the raw name.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("session name %q contains invalid characters: allowed characters are alphanumeric, spaces, hyphens, underscores, colons, periods, and slashes", name)
	}
	return nil
}

// Slugify converts a name to a URL-safe slug for use in directory paths and tmux session names.
// "My Session Name"    -> "my-session-name"
// "dev/test-thing" -> "dev-test-thing"
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// State represents the lifecycle state of a session.
type State string

const (
	StateActive    State = "active"
	StateRecycled  State = "recycled"
	StateCorrupted State = "corrupted"
)

// Metadata keys for terminal integration.
const (
	MetaTmuxSession = "tmux_session" // tmux session name
	MetaTmuxPane    = "tmux_pane"    // tmux pane identifier
)

// Metadata keys for session organization.
const (
	MetaGroup = "group" // user-assigned group for tree view grouping
)

// Clone strategy constants.
const (
	CloneStrategyFull     = "full"
	CloneStrategyWorktree = "worktree"
)

// Metadata keys for worktree sessions.
const (
	MetaWorktreeBranch = "worktree_branch" // branch name used by the git worktree
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
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	Path          string            `json:"path"`
	Remote        string            `json:"remote"`
	State         State             `json:"state"`
	CloneStrategy string            `json:"clone_strategy,omitempty"` // "full" (default) or "worktree"
	Metadata      map[string]string `json:"metadata,omitempty"`       // integration data (e.g., tmux session name)
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
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

// Group returns the user-assigned group for tree view organization, or empty string if unset.
func (s *Session) Group() string {
	return s.GetMeta(MetaGroup)
}

// SetGroup sets the user-assigned group for tree view organization.
// An empty value clears the group assignment.
func (s *Session) SetGroup(group string) {
	if group == "" {
		if s.Metadata != nil {
			delete(s.Metadata, MetaGroup)
		}
		return
	}
	s.SetMeta(MetaGroup, group)
}
