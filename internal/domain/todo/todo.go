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

// Validate checks that a Todo has internally consistent required fields.
func (t Todo) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("todo ID is required")
	}
	if t.Title == "" {
		return fmt.Errorf("todo title is required")
	}
	if !t.Source.IsValid() {
		return fmt.Errorf("invalid source %q", t.Source)
	}
	if !t.Status.IsValid() {
		return fmt.Errorf("invalid status %q", t.Status)
	}
	if t.Source == SourceAgent && t.SessionID == "" {
		return fmt.Errorf("session ID is required for agent-created todos")
	}
	if t.Source != SourceAgent && t.SessionID != "" {
		return fmt.Errorf("session ID is only valid for agent-created todos")
	}
	if !t.URI.IsEmpty() && !t.URI.Valid() {
		return fmt.Errorf("invalid URI %q: must use scheme://value format", t.URI.String())
	}
	return nil
}

func newTodoWithSession(id, title string, source Source, sessionID string, uri Ref) (Todo, error) {
	now := time.Now()
	t := Todo{
		ID:        id,
		SessionID: sessionID,
		Source:    source,
		Title:     title,
		URI:       uri,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := t.Validate(); err != nil {
		return Todo{}, err
	}
	return t, nil
}

// NewAgentTodo creates an agent-sourced todo and requires SessionID.
func NewAgentTodo(id, title, sessionID string, uri Ref) (Todo, error) {
	if sessionID == "" {
		return Todo{}, fmt.Errorf("session ID is required for agent-created todos")
	}
	return newTodoWithSession(id, title, SourceAgent, sessionID, uri)
}

// NewHumanTodo creates a human-sourced todo.
func NewHumanTodo(id, title string, uri Ref) (Todo, error) {
	return newTodoWithSession(id, title, SourceHuman, "", uri)
}

// NewSystemTodo creates a system-sourced todo.
func NewSystemTodo(id, title string, uri Ref) (Todo, error) {
	return newTodoWithSession(id, title, SourceSystem, "", uri)
}
