package stores

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/pkg/randid"
)

// TodoStore implements todo.Store using SQLite.
type TodoStore struct {
	db *db.DB
}

var _ todo.Store = (*TodoStore)(nil)

// NewTodoStore creates a new SQLite-backed TODO store.
func NewTodoStore(db *db.DB) *TodoStore {
	return &TodoStore{db: db}
}

// Create persists a new TODO item.
// Generates an ID if not set. Returns ErrDuplicate if a pending item already
// exists for the same file path (enforced by partial unique index).
func (s *TodoStore) Create(ctx context.Context, item todo.Item) error {
	if item.ID == "" {
		item.ID = randid.Generate(8)
	}

	now := time.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	if item.Status == "" {
		item.Status = todo.StatusPending
	}

	err := s.db.Queries().CreateTodoItem(ctx, db.CreateTodoItemParams{
		ID:          item.ID,
		Type:        string(item.Type),
		Status:      string(item.Status),
		Title:       item.Title,
		Description: toNullString(item.Description),
		FilePath:    toNullString(item.FilePath),
		SessionID:   toNullString(item.SessionID),
		RepoRemote:  item.RepoRemote,
		CreatedAt:   item.CreatedAt.UnixNano(),
		UpdatedAt:   item.UpdatedAt.UnixNano(),
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return todo.ErrDuplicate
		}
		return fmt.Errorf("create todo item: %w", err)
	}

	return nil
}

// Get returns a single TODO item by ID.
func (s *TodoStore) Get(ctx context.Context, id string) (todo.Item, error) {
	row, err := s.db.Queries().GetTodoItem(ctx, id)
	if err != nil {
		if IsNotFoundError(err) {
			return todo.Item{}, todo.ErrNotFound
		}
		return todo.Item{}, fmt.Errorf("get todo item: %w", err)
	}

	return rowToTodoItem(row), nil
}

// List returns TODO items matching the filter, ordered by created_at DESC.
func (s *TodoStore) List(ctx context.Context, filter todo.ListFilter) ([]todo.Item, error) {
	var rows []db.TodoItem
	var err error

	hasStatus := filter.Status != ""
	hasSession := filter.SessionID != ""
	hasRepo := filter.RepoRemote != ""

	switch {
	case hasStatus && hasSession:
		rows, err = s.db.Queries().ListTodoItemsByStatusAndSession(ctx, db.ListTodoItemsByStatusAndSessionParams{
			Status:    string(filter.Status),
			SessionID: toNullString(filter.SessionID),
		})
	case hasStatus && hasRepo:
		rows, err = s.db.Queries().ListTodoItemsByStatusAndRepo(ctx, db.ListTodoItemsByStatusAndRepoParams{
			Status:     string(filter.Status),
			RepoRemote: filter.RepoRemote,
		})
	case hasStatus:
		rows, err = s.db.Queries().ListTodoItemsByStatus(ctx, string(filter.Status))
	case hasSession:
		rows, err = s.db.Queries().ListTodoItemsBySession(ctx, toNullString(filter.SessionID))
	case hasRepo:
		rows, err = s.db.Queries().ListTodoItemsByRepo(ctx, filter.RepoRemote)
	default:
		rows, err = s.db.Queries().ListTodoItems(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("list todo items: %w", err)
	}

	items := make([]todo.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToTodoItem(row))
	}

	return items, nil
}

// UpdateStatus changes the status of a TODO item.
func (s *TodoStore) UpdateStatus(ctx context.Context, id string, status todo.Status) error {
	// Verify the item exists first
	_, err := s.db.Queries().GetTodoItem(ctx, id)
	if err != nil {
		if IsNotFoundError(err) {
			return todo.ErrNotFound
		}
		return fmt.Errorf("get todo item for update: %w", err)
	}

	err = s.db.Queries().UpdateTodoItemStatus(ctx, db.UpdateTodoItemStatusParams{
		Status:    string(status),
		UpdatedAt: time.Now().UnixNano(),
		ID:        id,
	})
	if err != nil {
		return fmt.Errorf("update todo item status: %w", err)
	}

	return nil
}

// DismissByPath dismisses all pending items matching the given file path.
func (s *TodoStore) DismissByPath(ctx context.Context, filePath string) error {
	err := s.db.Queries().DismissTodoItemsByPath(ctx, db.DismissTodoItemsByPathParams{
		UpdatedAt: time.Now().UnixNano(),
		FilePath:  toNullString(filePath),
	})
	if err != nil {
		return fmt.Errorf("dismiss todo items by path: %w", err)
	}

	return nil
}

// CountPending returns the total number of pending items.
func (s *TodoStore) CountPending(ctx context.Context) (int64, error) {
	count, err := s.db.Queries().CountPendingTodoItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("count pending todo items: %w", err)
	}
	return count, nil
}

// CountPendingBySession returns pending item count for a specific session.
func (s *TodoStore) CountPendingBySession(ctx context.Context, sessionID string) (int64, error) {
	count, err := s.db.Queries().CountPendingTodoItemsBySession(ctx, toNullString(sessionID))
	if err != nil {
		return 0, fmt.Errorf("count pending todo items by session: %w", err)
	}
	return count, nil
}

// CountCustomBySessionSince counts custom items created by a session since a given time.
func (s *TodoStore) CountCustomBySessionSince(ctx context.Context, sessionID string, since time.Time) (int64, error) {
	count, err := s.db.Queries().CountCustomTodoItemsBySessionSince(ctx, db.CountCustomTodoItemsBySessionSinceParams{
		SessionID: toNullString(sessionID),
		CreatedAt: since.UnixNano(),
	})
	if err != nil {
		return 0, fmt.Errorf("count custom todo items by session since: %w", err)
	}
	return count, nil
}

func rowToTodoItem(row db.TodoItem) todo.Item {
	return todo.Item{
		ID:          row.ID,
		Type:        todo.ItemType(row.Type),
		Status:      todo.Status(row.Status),
		Title:       row.Title,
		Description: fromNullString(row.Description),
		FilePath:    fromNullString(row.FilePath),
		SessionID:   fromNullString(row.SessionID),
		RepoRemote:  row.RepoRemote,
		CreatedAt:   time.Unix(0, row.CreatedAt),
		UpdatedAt:   time.Unix(0, row.UpdatedAt),
	}
}

// isUniqueConstraintError checks if the error is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
