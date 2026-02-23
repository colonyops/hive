package todo

import "time"

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
