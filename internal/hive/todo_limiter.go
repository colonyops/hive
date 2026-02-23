package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/todo"
)

// TodoLimiter enforces rate and capacity limits on todo creation.
type TodoLimiter struct {
	store        todo.Store
	maxPending   int
	rateLimitDur time.Duration
}

// NewTodoLimiter creates a limiter with the given configuration.
func NewTodoLimiter(store todo.Store, cfg config.TodosLimiterConfig) *TodoLimiter {
	return &TodoLimiter{
		store:        store,
		maxPending:   cfg.MaxPending,
		rateLimitDur: cfg.RateLimitPerSession,
	}
}

// Check returns nil if the todo is allowed, or an error describing why it was rejected.
func (l *TodoLimiter) Check(ctx context.Context, t todo.Todo) error {
	if l.maxPending > 0 {
		count, err := l.store.CountPending(ctx)
		if err != nil {
			return fmt.Errorf("check max pending: %w", err)
		}
		if count >= l.maxPending {
			return fmt.Errorf("max pending todos reached (%d)", l.maxPending)
		}
	}

	if l.rateLimitDur > 0 && t.SessionID != "" {
		since := time.Now().Add(-l.rateLimitDur)
		count, err := l.store.CountRecentBySession(ctx, t.SessionID, since)
		if err != nil {
			return fmt.Errorf("check rate limit: %w", err)
		}
		if count >= 1 {
			return fmt.Errorf("rate limited: session %q already created a todo within %s", t.SessionID, l.rateLimitDur)
		}
	}

	return nil
}
