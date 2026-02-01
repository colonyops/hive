package messaging

import (
	"context"
	"time"
)

// TopicEvent represents a change to a topic file.
type TopicEvent struct {
	Topic     string
	Timestamp time.Time
}

// Watcher watches for changes to topic files.
type Watcher interface {
	// Watch returns a channel that receives events when topics matching the pattern change.
	// Pattern supports:
	//   - "*" or "" matches all topics
	//   - "prefix.*" matches topics starting with "prefix."
	//   - exact topic name for single topic
	// The returned channel is closed when the context is cancelled or Close is called.
	Watch(ctx context.Context, pattern string) (<-chan TopicEvent, error)

	// Close stops all watching and releases resources.
	Close() error
}
