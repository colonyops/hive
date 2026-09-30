package stores

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// Seed exposes test fixtures without exposing generated queries outside
// internal/app/data.
type Seed struct {
	q *queries.DB
}

func NewSeed(q *queries.DB) Seed {
	return Seed{q: q}
}

// InboxItem ignores ID, revision, and triage fields not accepted by the
// insert query.
func (s Seed) InboxItem(ctx context.Context, item InboxItem) (InboxItem, error) {
	row, err := s.q.Ctx(ctx).InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: item.ProfileID, SourceKind: item.SourceKind, SourceScope: item.SourceScope, ExternalID: item.ExternalID,
		Title: item.Title, Url: item.URL, Payload: item.Payload, Unread: boolToInt64(item.Unread), Lifecycle: item.Lifecycle,
		FirstSeenAt: item.FirstSeenAt, LastEventAt: item.LastEventAt,
	})
	if err != nil {
		return InboxItem{}, wrap("seeding inbox item", err)
	}
	return mapInboxItemFromDB(row), nil
}

// InboxEvent ignores event.ID.
func (s Seed) InboxEvent(ctx context.Context, event InboxEvent) (InboxEvent, error) {
	row, err := s.q.Ctx(ctx).InsertInboxEvent(ctx, queries.InsertInboxEventParams{
		ItemID: event.ItemID, Kind: event.Kind, Transition: event.Transition, Attention: event.Attention,
		Summary: null(event.Summary), Detail: event.Detail, CreatedAt: event.CreatedAt,
	})
	if err != nil {
		return InboxEvent{}, wrap("seeding inbox event", err)
	}
	return mapInboxEventFromDB(row), nil
}

func (s Seed) ConsumerOffset(ctx context.Context, consumer string, offset int64) error {
	return wrap("seeding consumer offset", s.q.Ctx(ctx).CommitConsumerOffset(ctx, queries.CommitConsumerOffsetParams{
		Consumer: consumer, Offset: offset,
	}))
}
