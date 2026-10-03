// Package messaging provides inter-agent communication via pub/sub topics.
//
// # Inbox Convention
//
// Each hive session has a conventional inbox topic:
//
//	agent.<session-id>.inbox
//
// The "agent" prefix refers to the AI agent running in the session,
// not the session itself. This convention allows agents to discover
// each other's inboxes and send direct messages.
//
// # Future: Multi-Agent Addressing
//
// When hive supports multiple agents per session, inbox addressing will use:
//
//	agent.<session-id>.<agent-name>.inbox
//
// The current format will continue to work as an alias to the primary/default agent:
//
//	agent.<session-id>.inbox → agent.<session-id>.main.inbox
//
// Examples:
//
//	agent.26kj0c.inbox            // Current format (default agent)
//	agent.26kj0c.claude.inbox     // Future: Named agent "claude"
//	agent.26kj0c.test-runner.inbox // Future: Named agent "test-runner"
package messaging

import "time"

// Message represents a single message published to a topic.
type Message struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Payload   string    `json:"payload"`
	Sender    string    `json:"sender,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Topic represents a named channel for messages.
type Topic struct {
	Name      string    `json:"name"`
	Messages  []Message `json:"messages"`
	UpdatedAt time.Time `json:"updated_at"`
}
