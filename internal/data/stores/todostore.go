package stores

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/rs/zerolog/log"
)

var (
	fallbackSource = todo.SourceSystem
	fallbackStatus = todo.StatusPending
)

// TodoStore implements todo.Store using SQLite.
type TodoStore struct {
	db *db.DB
}

var _ todo.Store = (*TodoStore)(nil)

// NewTodoStore creates a new SQLite-backed todo store.
func NewTodoStore(db *db.DB) *TodoStore {
	return &TodoStore{db: db}
}

// Create persists a new todo item.
func (s *TodoStore) Create(ctx context.Context, t todo.Todo) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("validate todo %q: %w", t.ID, err)
	}

	err := s.db.Queries().CreateTodoItem(ctx, db.CreateTodoItemParams{
		ID:        t.ID,
		SessionID: t.SessionID,
		Source:    string(t.Source),
		Title:     t.Title,
		Uri:       t.URI.String(),
		Status:    string(t.Status),
		CreatedAt: t.CreatedAt.UnixNano(),
		UpdatedAt: t.UpdatedAt.UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("create todo item: %w", err)
	}

	return nil
}

// Get retrieves a single todo item by ID.
func (s *TodoStore) Get(ctx context.Context, id string) (todo.Todo, error) {
	row, err := s.db.Queries().GetTodoItem(ctx, id)
	if err != nil {
		return todo.Todo{}, fmt.Errorf("get todo item: %w", err)
	}
	return rowToTodo(row), nil
}

// Update changes the status of a todo item.
func (s *TodoStore) Update(ctx context.Context, id string, status todo.Status) error {
	now := time.Now()
	var completedAt int64
	if status == todo.StatusCompleted || status == todo.StatusDismissed {
		completedAt = now.UnixNano()
	}

	err := s.db.Queries().UpdateTodoItemStatus(ctx, db.UpdateTodoItemStatusParams{
		Status:      string(status),
		UpdatedAt:   now.UnixNano(),
		CompletedAt: completedAt,
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("update todo item status: %w", err)
	}
	return nil
}

// List returns todo items matching the given filter.
func (s *TodoStore) List(ctx context.Context, filter todo.ListFilter) ([]todo.Todo, error) {
	rows, err := s.listRows(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list todo items: %w", err)
	}

	result := make([]todo.Todo, 0, len(rows))
	for _, row := range rows {
		t := rowToTodo(row)
		if !matchesListFilter(t, filter) {
			continue
		}
		result = append(result, t)
	}

	return result, nil
}

func (s *TodoStore) listRows(ctx context.Context, filter todo.ListFilter) ([]db.TodoItem, error) {
	if filter.Status != nil {
		return s.db.Queries().ListTodoItemsByStatus(ctx, string(*filter.Status))
	}
	return s.db.Queries().ListTodoItems(ctx)
}

func matchesListFilter(item todo.Todo, filter todo.ListFilter) bool {
	if filter.SessionID != "" && item.SessionID != filter.SessionID {
		return false
	}
	if filter.Scheme != "" && item.URI.Scheme() != filter.Scheme {
		return false
	}
	return true
}

// CountPending returns the number of pending todo items.
func (s *TodoStore) CountPending(ctx context.Context) (int, error) {
	count, err := s.db.Queries().CountPendingTodoItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("count pending todo items: %w", err)
	}
	return int(count), nil
}

// CountOpen returns the number of open (pending + acknowledged) todo items.
func (s *TodoStore) CountOpen(ctx context.Context) (int, error) {
	count, err := s.db.Queries().CountOpenTodoItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("count open todo items: %w", err)
	}
	return int(count), nil
}

// CountRecentBySession returns the number of todo items created by a session since the given time.
func (s *TodoStore) CountRecentBySession(ctx context.Context, sessionID string, since time.Time) (int, error) {
	count, err := s.db.Queries().CountRecentTodoItemsBySession(ctx, db.CountRecentTodoItemsBySessionParams{
		SessionID: sessionID,
		CreatedAt: since.UnixNano(),
	})
	if err != nil {
		return 0, fmt.Errorf("count recent todo items by session: %w", err)
	}
	return int(count), nil
}

// Delete removes a todo item by ID.
func (s *TodoStore) Delete(ctx context.Context, id string) error {
	if err := s.db.Queries().DeleteTodoItem(ctx, id); err != nil {
		return fmt.Errorf("delete todo item: %w", err)
	}
	return nil
}

func rowToTodo(row db.TodoItem) todo.Todo {
	uri := parseStoredRef(row)
	source := parseStoredSource(row)
	status := parseStoredStatus(row)

	t := todo.Todo{
		ID:        row.ID,
		SessionID: row.SessionID,
		Source:    source,
		Title:     row.Title,
		URI:       uri,
		Status:    status,
		CreatedAt: time.Unix(0, row.CreatedAt),
		UpdatedAt: time.Unix(0, row.UpdatedAt),
	}
	if row.CompletedAt != 0 {
		t.CompletedAt = time.Unix(0, row.CompletedAt)
	}
	return t
}

func parseStoredRef(row db.TodoItem) todo.Ref {
	ref, err := todo.ParseRef(row.Uri)
	if err != nil {
		log.Debug().Err(err).Str("id", row.ID).Str("uri", row.Uri).Msg("invalid URI in stored todo")
		return todo.Ref{}
	}
	return ref
}

func parseStoredSource(row db.TodoItem) todo.Source {
	source, err := todo.ParseSource(row.Source)
	if err != nil {
		log.Warn().Err(err).Str("id", row.ID).Str("source", row.Source).Msg("invalid source in stored todo, defaulting to system")
		return fallbackSource
	}
	return source
}

func parseStoredStatus(row db.TodoItem) todo.Status {
	status, err := todo.ParseStatus(row.Status)
	if err != nil {
		log.Warn().Err(err).Str("id", row.ID).Str("status", row.Status).Msg("invalid status in stored todo, defaulting to pending")
		return fallbackStatus
	}
	return status
}
