package pipelinedb

import (
	"context"
	"encoding/json"
	"fmt"
)

// FeedItemView is the JSON/Wails-friendly shape of a persisted feed_item
// row. It is named "View" (rather than FeedItem) only to avoid colliding
// with the sqlc-generated raw row model of the same name in models.go —
// see FeedItems/MarkFeedItemRead below, and package pipeline's FeedItem
// alias, which re-exports this type under the name callers actually use.
type FeedItemView struct {
	FeedID    string          `json:"feedId"`
	ItemID    string          `json:"itemId"`
	Payload   json.RawMessage `json:"payload"`
	UpdatedAt int64           `json:"updatedAt"`
	Unread    bool            `json:"unread"`
}

// FeedItems returns the persisted items for feedID, newest first.
func (db *DB) FeedItems(ctx context.Context, feedID string) ([]FeedItemView, error) {
	rows, err := db.queries.ListFeedItemsByFeed(ctx, feedID)
	if err != nil {
		return nil, fmt.Errorf("listing feed items for feed %q: %w", feedID, err)
	}

	items := make([]FeedItemView, 0, len(rows))
	for _, row := range rows {
		items = append(items, FeedItemView{
			FeedID:    row.FeedID,
			ItemID:    row.ItemID,
			Payload:   json.RawMessage(row.Payload),
			UpdatedAt: row.UpdatedAt,
			Unread:    row.Unread != 0,
		})
	}
	return items, nil
}

// MarkFeedItemRead clears the unread flag on one feed item.
func (db *DB) MarkFeedItemRead(ctx context.Context, feedID, itemID string) error {
	if err := db.queries.MarkFeedItemRead(ctx, MarkFeedItemReadParams{
		FeedID: feedID,
		ItemID: itemID,
	}); err != nil {
		return fmt.Errorf("marking feed item %s/%s read: %w", feedID, itemID, err)
	}
	return nil
}
