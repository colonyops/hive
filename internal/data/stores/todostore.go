package stores

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
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

// Create persists a new todo item after validating enum fields.
func (s *TodoStore) Create(ctx context.Context, t todo.Todo) error {
	if _, err := todo.ParseSource(string(t.Source)); err != nil {
		return fmt.Errorf("invalid source %q: %w", t.Source, err)
	}
	if _, err := todo.ParseCategory(string(t.Category)); err != nil {
		return fmt.Errorf("invalid category %q: %w", t.Category, err)
	}
	if _, err := todo.ParseStatus(string(t.Status)); err != nil {
		return fmt.Errorf("invalid status %q: %w", t.Status, err)
	}

	err := s.db.Queries().CreateTodoItem(ctx, db.CreateTodoItemParams{
		ID:        t.ID,
		SessionID: t.SessionID,
		Source:    string(t.Source),
		Category:  string(t.Category),
		Title:     t.Title,
		Ref:       t.Ref,
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
	var rows []db.TodoItem
	var err error

	if filter.Status != nil {
		rows, err = s.db.Queries().ListTodoItemsByStatus(ctx, string(*filter.Status))
	} else {
		rows, err = s.db.Queries().ListTodoItems(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("list todo items: %w", err)
	}

	result := make([]todo.Todo, 0, len(rows))
	for _, row := range rows {
		t := rowToTodo(row)

		if filter.SessionID != "" && t.SessionID != filter.SessionID {
			continue
		}
		if filter.Category != nil && t.Category != *filter.Category {
			continue
		}

		result = append(result, t)
	}

	return result, nil
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
	t := todo.Todo{
		ID:        row.ID,
		SessionID: row.SessionID,
		Source:    todo.Source(row.Source),
		Category:  todo.Category(row.Category),
		Title:     row.Title,
		Ref:       row.Ref,
		Status:    todo.Status(row.Status),
		CreatedAt: time.Unix(0, row.CreatedAt),
		UpdatedAt: time.Unix(0, row.UpdatedAt),
	}
	if row.CompletedAt != 0 {
		t.CompletedAt = time.Unix(0, row.CompletedAt)
	}
	return t
}
