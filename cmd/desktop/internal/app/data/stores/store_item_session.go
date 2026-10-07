package stores

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type ItemSessionStore struct {
	q   *queries.DB
	now func() time.Time
}

func NewItemSessionStore(q *queries.DB, opts Options) *ItemSessionStore {
	return &ItemSessionStore{q: q, now: opts.Now}
}

// Persist only the association; hive owns mutable session details.
func (s *ItemSessionStore) Link(ctx context.Context, sessionID string, ref models.ItemRef) error {
	if sessionID == "" || !ref.Known() {
		return nil
	}
	err := s.q.Ctx(ctx).LinkItemSession(ctx, queries.LinkItemSessionParams{
		SessionID:   sessionID,
		ProfileID:   ref.ProfileID,
		SourceKind:  ref.SourceKind,
		SourceScope: ref.SourceScope,
		ExternalID:  ref.ExternalID,
		CreatedAt:   s.now().UnixMilli(),
	})
	return wrap("linking session to inbox item", err)
}

// Links may outlive hive sessions; callers reconcile them with Unlink.
func (s *ItemSessionStore) List(ctx context.Context, ref models.ItemRef) ([]ItemSession, error) {
	if !ref.Known() {
		return []ItemSession{}, nil
	}
	rows, err := s.q.Ctx(ctx).ListItemSessions(ctx, queries.ListItemSessionsParams(ref))
	if err != nil {
		return nil, wrap("listing sessions for an inbox item", err)
	}
	return MapFunc[queries.ItemSession, ItemSession](mapItemSessionFromDB).Slice(rows), nil
}

// Only unlink IDs absent from a successful hive session listing; an errored
// listing proves nothing.
func (s *ItemSessionStore) Unlink(ctx context.Context, sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	q := s.q.Ctx(ctx)
	for _, id := range sessionIDs {
		if err := q.DeleteItemSession(ctx, id); err != nil {
			return wrap("unlinking a session from its inbox item", err)
		}
	}
	return nil
}

// LinkChat records that an agent workspace chat was opened for an item.
// Deleting the chat removes the link with it.
func (s *ItemSessionStore) LinkChat(ctx context.Context, chatID string, ref models.ItemRef) error {
	if !ref.Known() {
		return nil
	}
	err := s.q.Ctx(ctx).LinkItemChat(ctx, queries.LinkItemChatParams{
		ChatID:      chatID,
		ProfileID:   ref.ProfileID,
		SourceKind:  ref.SourceKind,
		SourceScope: ref.SourceScope,
		ExternalID:  ref.ExternalID,
		CreatedAt:   s.now().UnixMilli(),
	})
	return wrap("linking chat to inbox item", err)
}

func (s *ItemSessionStore) ListChats(ctx context.Context, ref models.ItemRef) ([]ItemChat, error) {
	if !ref.Known() {
		return []ItemChat{}, nil
	}
	rows, err := s.q.Ctx(ctx).ListItemChats(ctx, queries.ListItemChatsParams(ref))
	if err != nil {
		return nil, wrap("listing chats for an inbox item", err)
	}
	return MapFunc[queries.ListItemChatsRow, ItemChat](mapItemChatFromDB).Slice(rows), nil
}

// DeleteByProfile removes links only; hive sessions and chats survive.
func (s *ItemSessionStore) DeleteByProfile(ctx context.Context, profileID string) error {
	return s.q.WithinTx(ctx, func(ctx context.Context, q *queries.DB) error {
		if err := q.DeleteItemSessionsByProfile(ctx, profileID); err != nil {
			return wrap("deleting item sessions by profile", err)
		}
		return wrap("deleting item chats by profile", q.DeleteItemChatsByProfile(ctx, profileID))
	})
}

// Empty-scope links must move with a migrated inbox row or they become
// unreachable.
func (s *ItemSessionStore) Rescope(ctx context.Context, profileID, sourceKind, externalID, scope string) error {
	err := s.q.WithinTx(ctx, func(ctx context.Context, q *queries.DB) error {
		if err := q.RescopeItemSessions(ctx, queries.RescopeItemSessionsParams{
			SourceScope: scope, ProfileID: profileID, SourceKind: sourceKind, ExternalID: externalID,
		}); err != nil {
			return err
		}
		if err := q.DeleteUnscopedItemSessions(ctx, queries.DeleteUnscopedItemSessionsParams{
			ProfileID: profileID, SourceKind: sourceKind, ExternalID: externalID,
		}); err != nil {
			return err
		}
		if err := q.RescopeItemChats(ctx, queries.RescopeItemChatsParams{
			SourceScope: scope, ProfileID: profileID, SourceKind: sourceKind, ExternalID: externalID,
		}); err != nil {
			return err
		}
		return q.DeleteUnscopedItemChats(ctx, queries.DeleteUnscopedItemChatsParams{
			ProfileID: profileID, SourceKind: sourceKind, ExternalID: externalID,
		})
	})
	return wrap("rescoping item sessions", err)
}
