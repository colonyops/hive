package hive

import (
	"context"
	"fmt"

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
	logger  zerolog.Logger
}

// NewTodoService creates a new TodoService.
func NewTodoService(store todo.Store, bus *eventbus.EventBus, cfg *config.Config, logger zerolog.Logger) *TodoService {
	return &TodoService{
		store:   store,
		limiter: NewTodoLimiter(store, cfg.Todos.Limiter),
		bus:     bus,
		logger:  logger.With().Str("component", "todo").Logger(),
	}
}

// Add creates a new todo item after passing validation and limiter checks.
func (s *TodoService) Add(ctx context.Context, t todo.Todo) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("invalid todo %q: %w", t.ID, err)
	}
	if t.Source == todo.SourceAgent && t.SessionID == "" {
		return fmt.Errorf("invalid todo %q: session ID is required for agent-created todos", t.ID)
	}

	if err := s.limiter.Check(ctx, t); err != nil {
		return fmt.Errorf("todo rejected: %w", err)
	}

	if err := s.store.Create(ctx, t); err != nil {
		return fmt.Errorf("create todo: %w", err)
	}

	s.bus.PublishTodoCreated(eventbus.TodoCreatedPayload{Todo: t})

	s.logger.Info().
		Str("id", t.ID).
		Str("uri", t.URI.String()).
		Str("session_id", t.SessionID).
		Msg("todo created")

	return nil
}

// validTransitions defines allowed status transitions.
var validTransitions = map[todo.Status][]todo.Status{
	todo.StatusPending:      {todo.StatusAcknowledged, todo.StatusCompleted, todo.StatusDismissed},
	todo.StatusAcknowledged: {todo.StatusCompleted, todo.StatusDismissed},
}

func validateTransition(from, to todo.Status) error {
	allowed := validTransitions[from]
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("invalid transition from %q to %q", from, to)
}

func (s *TodoService) transition(ctx context.Context, id string, to todo.Status, op string) error {
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get todo %q for %s: %w", id, op, err)
	}
	if err := validateTransition(current.Status, to); err != nil {
		return fmt.Errorf("%s todo %q: %w", op, id, err)
	}
	if err := s.store.Update(ctx, id, to); err != nil {
		return fmt.Errorf("%s todo %q: %w", op, id, err)
	}
	return nil
}

// Acknowledge updates a todo's status to acknowledged.
func (s *TodoService) Acknowledge(ctx context.Context, id string) error {
	return s.transition(ctx, id, todo.StatusAcknowledged, "acknowledge")
}

// Complete updates a todo's status to completed.
func (s *TodoService) Complete(ctx context.Context, id string) error {
	return s.transition(ctx, id, todo.StatusCompleted, "complete")
}

// Dismiss updates a todo's status to dismissed.
func (s *TodoService) Dismiss(ctx context.Context, id string) error {
	return s.transition(ctx, id, todo.StatusDismissed, "dismiss")
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

// CountOpen returns the number of open (pending + acknowledged) todo items.
func (s *TodoService) CountOpen(ctx context.Context) (int, error) {
	return s.store.CountOpen(ctx)
}
