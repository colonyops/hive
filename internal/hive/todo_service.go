package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/rs/zerolog"
)

// TodoService manages todo items with rate limiting.
type TodoService struct {
	store   todo.Store
	limiter *TodoLimiter
	bus     *eventbus.EventBus
	logger  zerolog.Logger
}

// NewTodoService creates a new TodoService.
func NewTodoService(store todo.Store, limiter *TodoLimiter, bus *eventbus.EventBus, logger zerolog.Logger) *TodoService {
	return &TodoService{
		store:   store,
		limiter: limiter,
		bus:     bus,
		logger:  logger,
	}
}

// Add creates a new todo item after checking rate limits and validation.
func (s *TodoService) Add(ctx context.Context, t todo.Todo) error {
	if err := s.limiter.Check(ctx, t.SessionID); err != nil {
		return err
	}

	if !t.URI.IsZero() && !t.URI.Valid() {
		return fmt.Errorf("invalid URI: must use scheme://value format")
	}

	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	t.Status = todo.StatusPending

	if err := s.store.Create(ctx, t); err != nil {
		return fmt.Errorf("create todo: %w", err)
	}

	s.logger.Info().
		Str("id", t.ID).
		Str("title", t.Title).
		Str("uri", t.URI.String()).
		Msg("todo created")

	if s.bus != nil {
		s.bus.PublishTodoCreated(eventbus.TodoCreatedPayload{
			Scheme: t.URI.Scheme,
			Title:  t.Title,
		})
	}

	return nil
}

// Acknowledge marks a todo as acknowledged.
func (s *TodoService) Acknowledge(ctx context.Context, id string) error {
	return s.store.Update(ctx, id, todo.StatusAcknowledged)
}

// Complete marks a todo as completed.
func (s *TodoService) Complete(ctx context.Context, id string) error {
	return s.store.Update(ctx, id, todo.StatusCompleted)
}

// Dismiss marks a todo as dismissed.
func (s *TodoService) Dismiss(ctx context.Context, id string) error {
	return s.store.Update(ctx, id, todo.StatusDismissed)
}

// List returns todos matching the filter.
func (s *TodoService) List(ctx context.Context, filter todo.ListFilter) ([]todo.Todo, error) {
	return s.store.List(ctx, filter)
}

// Get returns a single todo by ID.
func (s *TodoService) Get(ctx context.Context, id string) (todo.Todo, error) {
	return s.store.Get(ctx, id)
}

// CountPending returns the number of pending todos.
func (s *TodoService) CountPending(ctx context.Context) (int, error) {
	return s.store.CountPending(ctx)
}

// CountOpen returns the number of pending + acknowledged todos.
func (s *TodoService) CountOpen(ctx context.Context) (int, error) {
	return s.store.CountOpen(ctx)
}
