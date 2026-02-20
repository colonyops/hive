// Package eventbus provides a typed publish/subscribe event bus for
// cross-component communication within hive.
package eventbus

import (
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/todo"
)

//go:generate gobusgen generate -p .Events

// Events defines all event types and their payload structs for code generation.
var Events = map[string]any{
	// Keep list sorted A-Z
	"agent.status-changed": AgentStatusChangedPayload{},
	"config.reloaded":      ConfigReloadedPayload{},
	"message.received":     MessageReceivedPayload{},
	"session.corrupted":    SessionCorruptedPayload{},
	"session.created":      SessionCreatedPayload{},
	"session.deleted":      SessionDeletedPayload{},
	"session.recycled":     SessionRecycledPayload{},
	"session.renamed":      SessionRenamedPayload{},
	"todo.created":         TodoCreatedPayload{},
	"todo.dismissed":       TodoDismissedPayload{},
	"tui.started":          TUIStartedPayload{},
	"tui.stopped":          TUIStoppedPayload{},
}

// SessionCreatedPayload is emitted when a new session is created.
type SessionCreatedPayload struct {
	Session *session.Session
}

// SessionRecycledPayload is emitted when a session is recycled.
type SessionRecycledPayload struct {
	Session *session.Session
}

// SessionDeletedPayload is emitted when a session is deleted.
type SessionDeletedPayload struct {
	SessionID string
}

// SessionRenamedPayload is emitted when a session is renamed.
type SessionRenamedPayload struct {
	Session *session.Session
	OldName string
}

// SessionCorruptedPayload is emitted when a session is marked corrupted.
type SessionCorruptedPayload struct {
	Session *session.Session
}

// AgentStatusChangedPayload is emitted when an agent's terminal status changes.
type AgentStatusChangedPayload struct {
	Session   *session.Session
	OldStatus terminal.Status
	NewStatus terminal.Status
}

// MessageReceivedPayload is emitted when a message is received on a topic.
type MessageReceivedPayload struct {
	Topic   string
	Message *messaging.Message
}

// TUIStartedPayload is emitted when the TUI starts.
type TUIStartedPayload struct{}

// TUIStoppedPayload is emitted when the TUI stops.
type TUIStoppedPayload struct{}

// ConfigReloadedPayload is emitted when configuration is reloaded.
type ConfigReloadedPayload struct {
	Config *config.Config
}

// TodoCreatedPayload is emitted when a new TODO item is created.
type TodoCreatedPayload struct {
	Item *todo.Item
}

// TodoDismissedPayload is emitted when a TODO item is dismissed.
type TodoDismissedPayload struct {
	Item *todo.Item
}
