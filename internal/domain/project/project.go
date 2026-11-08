// Package project defines project sessions: one agent root over several
// ordinary hive sessions, one per repository.
package project

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrNotFound      = errors.New("project session not found")
	ErrDuplicateName = errors.New("project session name already in use")
)

// Session is a project session. It owns Root, a generated directory with one
// symlink per repository, and the child hive sessions those links point at.
// The children stay ordinary sessions: git status, diffs, and recycling read
// them the way they read any session.
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Project   string    `json:"project,omitempty"`
	Root      string    `json:"root"`
	Repos     []Repo    `json:"repos"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Repo is one repository of a project session.
type Repo struct {
	// Dir is the name of the symlink in Root.
	Dir       string `json:"dir"`
	SessionID string `json:"session_id"`
}

// Store persists project sessions.
type Store interface {
	List(ctx context.Context) ([]Session, error)
	Get(ctx context.Context, id string) (Session, error)
	Save(ctx context.Context, s Session) error
	Delete(ctx context.Context, id string) error
	// ProjectOf returns the project session that holds the hive session, and
	// false when it is in none.
	ProjectOf(ctx context.Context, sessionID string) (Session, bool, error)
}

var dirPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateDir checks that dir is a plain directory name.
func ValidateDir(dir string) error {
	if !dirPattern.MatchString(dir) || dir == "." || dir == ".." {
		return fmt.Errorf("repo dir %q must be a plain directory name", dir)
	}
	switch dir {
	case "AGENTS.md", "CLAUDE.md":
		return fmt.Errorf("repo dir %q clashes with a generated file", dir)
	}
	return nil
}
