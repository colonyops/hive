// Package session defines session domain types and interfaces.
package session

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Validation errors for Session.
var (
	ErrEmptyID     = errors.New("id is required")
	ErrEmptyName   = errors.New("name is required")
	ErrEmptyPath   = errors.New("path is required")
	ErrEmptyRemote = errors.New("remote is required")
	ErrInvalidState = errors.New("invalid state")
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
type State string

const (
	StateActive    State = "active"
	StateRecycled  State = "recycled"
	StateCorrupted State = "corrupted"
)

// IsValid returns true if s is a known state.
func (s State) IsValid() bool {
	switch s {
	case StateActive, StateRecycled, StateCorrupted:
		return true
	}
	return false
}

// Session represents an isolated git environment for an AI agent.
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Path      string    `json:"path"`
	Remote    string    `json:"remote"`
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewSession creates a new Session with the given parameters.
// Sets initial state to Active and timestamps to now.
// Returns an error if validation fails.
func NewSession(id, name, path, remote string, now time.Time) (Session, error) {
	s := Session{
		ID:        id,
		Name:      name,
		Slug:      Slugify(name),
		Path:      path,
		Remote:    remote,
		State:     StateActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.Validate(); err != nil {
		return Session{}, err
	}
	return s, nil
}

// Validate checks that the session meets all constraints.
func (s *Session) Validate() error {
	if s.ID == "" {
		return ErrEmptyID
	}
	if s.Name == "" {
		return ErrEmptyName
	}
	if s.Path == "" {
		return ErrEmptyPath
	}
	if s.Remote == "" {
		return ErrEmptyRemote
	}
	if !s.State.IsValid() {
		return ErrInvalidState
	}
	return nil
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
