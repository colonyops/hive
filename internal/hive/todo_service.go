package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/rs/zerolog"
)

// TodoService orchestrates todo operations with rate limiting and event publishing.
type TodoService struct {
	store   todo.Store
	limiter *TodoLimiter
	bus     *eventbus.EventBus
	mode    string
	logger  zerolog.Logger
}

// NewTodoService creates a new TodoService.
func NewTodoService(store todo.Store, bus *eventbus.EventBus, cfg *config.Config, logger zerolog.Logger) *TodoService {
	return &TodoService{
		store:   store,
		limiter: NewTodoLimiter(store, cfg.Todos.Limiter),
		bus:     bus,
		mode:    cfg.Todos.Mode,
		logger:  logger.With().Str("component", "todo").Logger(),
	}
}

// Add creates a new todo item after passing limiter checks.
func (s *TodoService) Add(ctx context.Context, t todo.Todo) error {
	if err := s.limiter.Check(ctx, t); err != nil {
		return fmt.Errorf("todo rejected: %w", err)
	}

	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	t.Status = todo.StatusPending

	if err := s.store.Create(ctx, t); err != nil {
		return fmt.Errorf("create todo: %w", err)
	}

	s.bus.PublishTodoCreated(eventbus.TodoCreatedPayload{Todo: t})

	s.logger.Info().
		Str("id", t.ID).
		Str("category", string(t.Category)).
		Str("session_id", t.SessionID).
		Msg("todo created")

	return nil
}

// Acknowledge updates a todo's status to acknowledged.
func (s *TodoService) Acknowledge(ctx context.Context, id string) error {
	if err := s.store.Update(ctx, id, todo.StatusAcknowledged); err != nil {
		return fmt.Errorf("acknowledge todo: %w", err)
	}
	return nil
}

// Complete updates a todo's status to completed.
func (s *TodoService) Complete(ctx context.Context, id string) error {
	if err := s.store.Update(ctx, id, todo.StatusCompleted); err != nil {
		return fmt.Errorf("complete todo: %w", err)
	}
	return nil
}

// Dismiss updates a todo's status to dismissed.
func (s *TodoService) Dismiss(ctx context.Context, id string) error {
	if err := s.store.Update(ctx, id, todo.StatusDismissed); err != nil {
		return fmt.Errorf("dismiss todo: %w", err)
	}
	return nil
}

// List returns todo items matching the given filter.
func (s *TodoService) List(ctx context.Context, filter todo.ListFilter) ([]todo.Todo, error) {
	return s.store.List(ctx, filter)
}

// Get retrieves a single todo item by ID.
func (s *TodoService) Get(ctx context.Context, id string) (todo.Todo, error) {
	return s.store.Get(ctx, id)
}

// CountPending returns the number of pending todo items.
func (s *TodoService) CountPending(ctx context.Context) (int, error) {
	return s.store.CountPending(ctx)
}

// Mode returns the configured todo mode ("internal" or "export-only").
func (s *TodoService) Mode() string {
	return s.mode
}
