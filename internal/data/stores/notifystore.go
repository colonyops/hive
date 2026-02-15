package stores

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/data/db"
)

// NotifyStore implements notify.Store using SQLite.
type NotifyStore struct {
	db *db.DB
}

var _ notify.Store = (*NotifyStore)(nil)

// NewNotifyStore creates a new SQLite-backed notification store.
func NewNotifyStore(db *db.DB) *NotifyStore {
	return &NotifyStore{db: db}
}

// Save persists a notification and returns its auto-generated ID.
func (s *NotifyStore) Save(ctx context.Context, n notify.Notification) (int64, error) {
	id, err := s.db.Queries().InsertNotification(ctx, db.InsertNotificationParams{
		Level:     string(n.Level),
		Message:   n.Message,
		CreatedAt: n.CreatedAt.UnixNano(),
	})
	if err != nil {
		return 0, fmt.Errorf("insert notification: %w", err)
	}

	return id, nil
}

// List returns all notifications ordered by newest first.
func (s *NotifyStore) List(ctx context.Context) ([]notify.Notification, error) {
	rows, err := s.db.Queries().ListNotifications(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}

	result := make([]notify.Notification, 0, len(rows))
	for _, row := range rows {
		result = append(result, rowToNotification(row))
	}

	return result, nil
}

// Clear deletes all notifications.
func (s *NotifyStore) Clear(ctx context.Context) error {
	if err := s.db.Queries().DeleteAllNotifications(ctx); err != nil {
		return fmt.Errorf("clear notifications: %w", err)
	}
	return nil
}

// Count returns the total number of notifications.
func (s *NotifyStore) Count(ctx context.Context) (int64, error) {
	count, err := s.db.Queries().CountNotifications(ctx)
	if err != nil {
		return 0, fmt.Errorf("count notifications: %w", err)
	}
	return count, nil
}

func rowToNotification(row db.Notification) notify.Notification {
	return notify.Notification{
		ID:        row.ID,
		Level:     notify.Level(row.Level),
		Message:   row.Message,
		CreatedAt: time.Unix(0, row.CreatedAt),
	}
}
