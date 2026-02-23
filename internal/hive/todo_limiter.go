package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/todo"
)

// TodoLimiter enforces rate limits on todo creation.
type TodoLimiter struct {
	store        todo.Store
	maxPending   int
	rateLimitDur time.Duration
}

// NewTodoLimiter creates a limiter with the given config.
func NewTodoLimiter(store todo.Store, cfg config.TodosLimiterConfig) *TodoLimiter {
	return &TodoLimiter{
		store:        store,
		maxPending:   cfg.MaxPending,
		rateLimitDur: cfg.RateLimitPerSession,
	}
}

// Check returns nil if the todo is allowed, or an error describing why it was rejected.
func (l *TodoLimiter) Check(ctx context.Context, sessionID string) error {
	pending, err := l.store.CountPending(ctx)
	if err != nil {
		return fmt.Errorf("check pending count: %w", err)
	}
	if pending >= l.maxPending {
		return fmt.Errorf("max pending todos reached (%d)", l.maxPending)
	}

	if sessionID != "" && l.rateLimitDur > 0 {
		since := time.Now().Add(-l.rateLimitDur)
		recent, err := l.store.CountRecentBySession(ctx, sessionID, since)
		if err != nil {
			return fmt.Errorf("check rate limit: %w", err)
		}
		if recent >= 1 {
			return fmt.Errorf("rate limited: session %q created a todo within the last %s", sessionID, l.rateLimitDur)
		}
	}

	return nil
}
