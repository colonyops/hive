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
	store    todo.Store
	limiter  *TodoLimiter
	bus      *eventbus.EventBus
	exporter *TodoExporter
	mode     string
	logger   zerolog.Logger
}

// NewTodoService creates a new TodoService.
func NewTodoService(store todo.Store, bus *eventbus.EventBus, cfg *config.Config, logger zerolog.Logger) *TodoService {
	svc := &TodoService{
		store:   store,
		limiter: NewTodoLimiter(store, cfg.Todos.Limiter),
		bus:     bus,
		mode:    cfg.Todos.Mode,
		logger:  logger.With().Str("component", "todo").Logger(),
	}

	if cfg.Todos.Export.Enabled {
		exp, err := NewTodoExporter(cfg.Todos.Export, logger)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to initialize todo exporter, export disabled")
		} else {
			svc.exporter = exp
		}
	}

	return svc
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

	// Export if enabled
	if s.exporter != nil {
		if err := s.exporter.Export([]todo.Todo{t}); err != nil {
			s.logger.Warn().Err(err).Str("id", t.ID).Msg("failed to export todo")
			if s.mode == "export-only" {
				return fmt.Errorf("export todo: %w", err)
			}
		} else if s.mode == "export-only" {
			// Auto-finalize after successful export
			if err := s.store.Update(ctx, t.ID, todo.StatusCompleted); err != nil {
				s.logger.Warn().Err(err).Str("id", t.ID).Msg("failed to auto-finalize exported todo")
			}
		}
	}

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

// CountOpen returns the number of open (pending + acknowledged) todo items.
func (s *TodoService) CountOpen(ctx context.Context) (int, error) {
	return s.store.CountOpen(ctx)
}

// Mode returns the configured todo mode ("internal" or "export-only").
func (s *TodoService) Mode() string {
	return s.mode
}
