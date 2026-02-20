package hive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/rs/zerolog"
)

// TodoService wraps todo.Store with domain logic for file-change detection,
// frontmatter parsing, deduplication, and event publishing.
type TodoService struct {
	store        todo.Store
	bus          *eventbus.EventBus
	log          zerolog.Logger
	rateLimit    int // max custom items per session per hour (0 = unlimited)
	rateDuration time.Duration
}

// NewTodoService creates a new TodoService.
func NewTodoService(store todo.Store, bus *eventbus.EventBus, log zerolog.Logger) *TodoService {
	return &TodoService{
		store:        store,
		bus:          bus,
		log:          log.With().Str("component", "todo-service").Logger(),
		rateLimit:    20,
		rateDuration: time.Hour,
	}
}

// HandleFileEvent processes a file change event from a context directory.
// It reads the file's frontmatter for session attribution and creates a TODO item.
// Duplicate pending items for the same file path are silently ignored.
func (s *TodoService) HandleFileEvent(ctx context.Context, filePath string, repoRemote string) error {
	fm := s.readFrontmatter(filePath)

	title := fm.Title
	if title == "" {
		title = filepath.Base(filePath)
	}

	item := todo.Item{
		Type:       todo.ItemTypeFileChange,
		Title:      title,
		FilePath:   filePath,
		SessionID:  fm.SessionID,
		RepoRemote: repoRemote,
	}

	if err := s.store.Create(ctx, item); err != nil {
		if errors.Is(err, todo.ErrDuplicate) {
			s.log.Debug().Str("path", filePath).Msg("duplicate pending todo, skipping")
			return nil
		}
		return fmt.Errorf("create todo for file event: %w", err)
	}

	s.bus.PublishTodoCreated(eventbus.TodoCreatedPayload{Item: &item})

	return nil
}

// HandleFileDelete dismisses all pending TODO items for a deleted file.
func (s *TodoService) HandleFileDelete(ctx context.Context, filePath string) error {
	if err := s.store.DismissByPath(ctx, filePath); err != nil {
		return fmt.Errorf("dismiss todo for deleted file: %w", err)
	}
	return nil
}

// CreateCustom creates a custom TODO item, subject to rate limiting per session.
func (s *TodoService) CreateCustom(ctx context.Context, item todo.Item) error {
	item.Type = todo.ItemTypeCustom

	if s.rateLimit > 0 && item.SessionID != "" {
		since := time.Now().Add(-s.rateDuration)
		count, err := s.store.CountCustomBySessionSince(ctx, item.SessionID, since)
		if err != nil {
			return fmt.Errorf("check rate limit: %w", err)
		}
		if count >= int64(s.rateLimit) {
			return todo.ErrRateLimited
		}
	}

	if err := s.store.Create(ctx, item); err != nil {
		return fmt.Errorf("create custom todo: %w", err)
	}

	s.bus.PublishTodoCreated(eventbus.TodoCreatedPayload{Item: &item})

	return nil
}

// Dismiss marks a TODO item as dismissed.
func (s *TodoService) Dismiss(ctx context.Context, id string) error {
	if err := s.store.UpdateStatus(ctx, id, todo.StatusDismissed); err != nil {
		return fmt.Errorf("dismiss todo: %w", err)
	}
	return nil
}

// DismissByPath dismisses all pending items for a given file path.
func (s *TodoService) DismissByPath(ctx context.Context, filePath string) error {
	return s.store.DismissByPath(ctx, filePath)
}

// Complete marks a TODO item as completed.
func (s *TodoService) Complete(ctx context.Context, id string) error {
	if err := s.store.UpdateStatus(ctx, id, todo.StatusCompleted); err != nil {
		return fmt.Errorf("complete todo: %w", err)
	}
	return nil
}

// CompleteByPath completes all pending items matching the given file path.
// This is used when a review is finalized for a specific document.
func (s *TodoService) CompleteByPath(ctx context.Context, filePath string) error {
	items, err := s.store.List(ctx, todo.ListFilter{Status: todo.StatusPending})
	if err != nil {
		return fmt.Errorf("list pending for complete by path: %w", err)
	}

	for _, item := range items {
		if item.FilePath == filePath {
			if err := s.store.UpdateStatus(ctx, item.ID, todo.StatusCompleted); err != nil {
				return fmt.Errorf("complete todo %s: %w", item.ID, err)
			}
		}
	}

	return nil
}

// ListPending returns all pending TODO items, optionally filtered.
func (s *TodoService) ListPending(ctx context.Context, filter todo.ListFilter) ([]todo.Item, error) {
	filter.Status = todo.StatusPending
	return s.store.List(ctx, filter)
}

// CountPending returns the total number of pending items.
func (s *TodoService) CountPending(ctx context.Context) (int64, error) {
	return s.store.CountPending(ctx)
}

// CountPendingBySession returns the pending item count for a specific session.
func (s *TodoService) CountPendingBySession(ctx context.Context, sessionID string) (int64, error) {
	return s.store.CountPendingBySession(ctx, sessionID)
}

// Get returns a single TODO item by ID.
func (s *TodoService) Get(ctx context.Context, id string) (todo.Item, error) {
	return s.store.Get(ctx, id)
}

// List returns TODO items matching the filter.
func (s *TodoService) List(ctx context.Context, filter todo.ListFilter) ([]todo.Item, error) {
	return s.store.List(ctx, filter)
}

// readFrontmatter reads and parses frontmatter from a file. Best-effort.
func (s *TodoService) readFrontmatter(filePath string) todo.Frontmatter {
	data, err := os.ReadFile(filePath)
	if err != nil {
		s.log.Debug().Err(err).Str("path", filePath).Msg("could not read file for frontmatter")
		return todo.Frontmatter{}
	}
	return todo.ParseFrontmatter(string(data))
}
