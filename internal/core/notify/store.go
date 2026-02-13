package notify

import (
	"context"
	"time"
)

// Level represents the severity of a notification.
type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
)

// Notification represents a single notification event.
type Notification struct {
	ID        int64
	Level     Level
	Message   string
	CreatedAt time.Time
}

// Store persists notifications to durable storage.
type Store interface {
	Save(ctx context.Context, n Notification) (int64, error)
	List(ctx context.Context) ([]Notification, error)
	Clear(ctx context.Context) error
	Count(ctx context.Context) (int64, error)
}
