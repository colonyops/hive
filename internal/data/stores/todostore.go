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

// Create persists a new todo item. The caller must set the ID.
func (s *TodoStore) Create(ctx context.Context, t todo.Todo) error {
	if _, err := todo.ParseSource(string(t.Source)); err != nil {
		return fmt.Errorf("invalid source: %w", err)
	}
	if _, err := todo.ParseStatus(string(t.Status)); err != nil {
		return fmt.Errorf("invalid status: %w", err)
	}

	return s.db.Queries().CreateTodoItem(ctx, db.CreateTodoItemParams{
		ID:        t.ID,
		SessionID: t.SessionID,
		Source:    string(t.Source),
		Title:     t.Title,
		Uri:       t.URI.String(),
		Status:    string(t.Status),
		CreatedAt: t.CreatedAt.UnixNano(),
		UpdatedAt: t.UpdatedAt.UnixNano(),
	})
}

// Get returns a todo by ID. Returns todo.ErrNotFound if not found.
func (s *TodoStore) Get(ctx context.Context, id string) (todo.Todo, error) {
	row, err := s.db.Queries().GetTodoItem(ctx, id)
	if IsNotFoundError(err) {
		return todo.Todo{}, todo.ErrNotFound
	}
	if err != nil {
		return todo.Todo{}, fmt.Errorf("get todo: %w", err)
	}

	return rowToTodo(row), nil
}

// Update changes the status of a todo item.
func (s *TodoStore) Update(ctx context.Context, id string, status todo.Status) error {
	if _, err := todo.ParseStatus(string(status)); err != nil {
		return fmt.Errorf("invalid status: %w", err)
	}

	var completedAt int64
	if status == todo.StatusCompleted || status == todo.StatusDismissed {
		completedAt = time.Now().UnixNano()
	}

	return s.db.Queries().UpdateTodoItemStatus(ctx, db.UpdateTodoItemStatusParams{
		Status:      string(status),
		UpdatedAt:   time.Now().UnixNano(),
		CompletedAt: completedAt,
		ID:          id,
	})
}

// List returns todos matching the filter, ordered by newest first.
// Filtering is done in Go post-fetch since the todo table is small.
func (s *TodoStore) List(ctx context.Context, filter todo.ListFilter) ([]todo.Todo, error) {
	var rows []db.TodoItem
	var err error

	if filter.Status != nil {
		rows, err = s.db.Queries().ListTodoItemsByStatus(ctx, string(*filter.Status))
	} else {
		rows, err = s.db.Queries().ListTodoItems(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}

	result := make([]todo.Todo, 0, len(rows))
	for _, row := range rows {
		t := rowToTodo(row)

		if filter.SessionID != "" && t.SessionID != filter.SessionID {
			continue
		}
		if filter.Scheme != "" && t.URI.Scheme != filter.Scheme {
			continue
		}

		result = append(result, t)
	}

	return result, nil
}

// CountPending returns the number of pending todos.
func (s *TodoStore) CountPending(ctx context.Context) (int, error) {
	count, err := s.db.Queries().CountPendingTodoItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("count pending todos: %w", err)
	}
	return int(count), nil
}

// CountOpen returns the number of pending + acknowledged todos.
func (s *TodoStore) CountOpen(ctx context.Context) (int, error) {
	count, err := s.db.Queries().CountOpenTodoItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("count open todos: %w", err)
	}
	return int(count), nil
}

// CountRecentBySession returns the number of todos created by a session since the given time.
func (s *TodoStore) CountRecentBySession(ctx context.Context, sessionID string, since time.Time) (int, error) {
	count, err := s.db.Queries().CountRecentTodoItemsBySession(ctx, db.CountRecentTodoItemsBySessionParams{
		SessionID: sessionID,
		CreatedAt: since.UnixNano(),
	})
	if err != nil {
		return 0, fmt.Errorf("count recent todos by session: %w", err)
	}
	return int(count), nil
}

// Delete removes a todo by ID.
func (s *TodoStore) Delete(ctx context.Context, id string) error {
	if err := s.db.Queries().DeleteTodoItem(ctx, id); err != nil {
		return fmt.Errorf("delete todo: %w", err)
	}
	return nil
}

func rowToTodo(row db.TodoItem) todo.Todo {
	t := todo.Todo{
		ID:        row.ID,
		SessionID: row.SessionID,
		Source:    todo.Source(row.Source),
		Title:     row.Title,
		URI:       todo.ParseURI(row.Uri),
		Status:    todo.Status(row.Status),
		CreatedAt: time.Unix(0, row.CreatedAt),
		UpdatedAt: time.Unix(0, row.UpdatedAt),
	}
	if row.CompletedAt != 0 {
		t.CompletedAt = time.Unix(0, row.CompletedAt)
	}
	return t
}
