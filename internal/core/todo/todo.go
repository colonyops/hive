package todo

import (
	"fmt"
	"time"
)

// Source identifies who created the todo.
//
// ENUM(
//
//	agent
//	human
//	system
//
// )
type Source string

// Status tracks the lifecycle of a todo item.
//
// ENUM(
//
//	pending
//	acknowledged
//	completed
//	dismissed
//
// )
type Status string

// Todo represents a single todo item created by an agent or human.
type Todo struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Source      Source    `json:"source"`
	Title       string    `json:"title"`
	URI         Ref       `json:"uri"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt time.Time `json:"completed_at,omitzero"`
}

// NewTodo creates a validated Todo with defaults for status and timestamps.
// ID and title are required. Source must be a valid enum value.
// If uri is non-empty, it must have a scheme.
func NewTodo(id, title string, source Source, uri Ref) (Todo, error) {
	if id == "" {
		return Todo{}, fmt.Errorf("todo ID is required")
	}
	if title == "" {
		return Todo{}, fmt.Errorf("todo title is required")
	}
	if !source.IsValid() {
		return Todo{}, fmt.Errorf("invalid source %q", source)
	}
	if !uri.IsEmpty() && !uri.Valid() {
		return Todo{}, fmt.Errorf("invalid URI %q: must use scheme://value format", uri.String())
	}

	now := time.Now()
	return Todo{
		ID:        id,
		Source:    source,
		Title:     title,
		URI:       uri,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
