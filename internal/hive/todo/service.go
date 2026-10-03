// Package todo is the todo service and its creation limiter.
package todo

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/todo"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/rs/zerolog"
)

// Service orchestrates todo operations with rate limiting and event publishing.
type Service struct {
	store   todo.Store
	limiter *Limiter
	bus     *events.EventBus
	logger  zerolog.Logger
}

// NewService creates a new Service.
func NewService(store todo.Store, bus *events.EventBus, cfg *config.Config, logger zerolog.Logger) *Service {
	return &Service{
		store:   store,
		limiter: NewLimiter(store, cfg.Todos.Limiter),
		bus:     bus,
		logger:  logger.With().Str("component", "todo").Logger(),
	}
}

// Add creates a new todo item after passing validation and limiter checks.
func (s *Service) Add(ctx context.Context, t todo.Todo) (todo.Todo, error) {
	if err := t.Validate(); err != nil {
		return todo.Todo{}, fmt.Errorf("invalid todo %q: %w", t.ID, err)
	}

	if err := s.limiter.Check(ctx, t); err != nil {
		return todo.Todo{}, fmt.Errorf("todo rejected: %w", err)
	}

	if err := s.store.Create(ctx, t); err != nil {
		return todo.Todo{}, fmt.Errorf("create todo: %w", err)
	}

	s.bus.PublishTodoCreated(events.TodoCreatedPayload{Todo: t})

	s.logger.Info().
		Str("id", t.ID).
		Str("uri", t.URI.String()).
		Str("session_id", t.SessionID).
		Msg("todo created")

	return t, nil
}

func (s *Service) transition(ctx context.Context, id string, to todo.Status, op string) (todo.Todo, error) {
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return todo.Todo{}, fmt.Errorf("get todo %q for %s: %w", id, op, err)
	}
	if err := todo.ValidateTransition(current.Status, to); err != nil {
		return todo.Todo{}, fmt.Errorf("%s todo %q: %w", op, id, err)
	}

	now := time.Now()
	if err := s.store.Update(ctx, id, to); err != nil {
		return todo.Todo{}, fmt.Errorf("%s todo %q: %w", op, id, err)
	}

	current.Status = to
	current.UpdatedAt = now
	if to == todo.StatusCompleted || to == todo.StatusDismissed {
		current.CompletedAt = now
	}

	return current, nil
}

// Acknowledge updates a todo's status to acknowledged.
func (s *Service) Acknowledge(ctx context.Context, id string) (todo.Todo, error) {
	return s.transition(ctx, id, todo.StatusAcknowledged, "acknowledge")
}

// Complete updates a todo's status to completed.
func (s *Service) Complete(ctx context.Context, id string) (todo.Todo, error) {
	return s.transition(ctx, id, todo.StatusCompleted, "complete")
}

// Dismiss updates a todo's status to dismissed.
func (s *Service) Dismiss(ctx context.Context, id string) (todo.Todo, error) {
	return s.transition(ctx, id, todo.StatusDismissed, "dismiss")
}

// Reopen reverts a completed or dismissed todo back to acknowledged.
func (s *Service) Reopen(ctx context.Context, id string) (todo.Todo, error) {
	return s.transition(ctx, id, todo.StatusAcknowledged, "reopen")
}

// List returns todo items matching the given filter.
func (s *Service) List(ctx context.Context, filter todo.ListFilter) ([]todo.Todo, error) {
	return s.store.List(ctx, filter)
}

// Get retrieves a single todo item by ID.
func (s *Service) Get(ctx context.Context, id string) (todo.Todo, error) {
	return s.store.Get(ctx, id)
}

// CountPending returns the number of pending todo items.
func (s *Service) CountPending(ctx context.Context) (int, error) {
	return s.store.CountPending(ctx)
}

// CountOpen returns the number of open (pending + acknowledged) todo items.
func (s *Service) CountOpen(ctx context.Context) (int, error) {
	return s.store.CountOpen(ctx)
}
