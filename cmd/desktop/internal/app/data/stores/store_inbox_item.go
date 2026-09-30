package stores

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// InboxItemStore owns inbox_item and inbox_event; ingest and scoped
// resolution coordinate sibling stores in the same transaction.
type InboxItemStore struct {
	q        *queries.DB
	heads    *SourceHeadStore
	sessions *ItemSessionStore
	// log is set by New after both stores exist: EventLogStore.Commit and
	// ActivateReplay resolve and mint through InboxItemStore, so neither
	// store can be fully built before the other.
	log    *EventLogStore
	now    func() time.Time
	mapper MapFunc[queries.InboxItem, InboxItem]
}

func NewInboxItemStore(q *queries.DB, opts Options, heads *SourceHeadStore, sessions *ItemSessionStore) *InboxItemStore {
	return &InboxItemStore{q: q, heads: heads, sessions: sessions, now: opts.Now, mapper: mapInboxItemFromDB}
}

func (s *InboxItemStore) ListByFeed(ctx context.Context, profileID, feedID string, limit int) ([]InboxItem, error) {
	if limit <= 0 {
		return []InboxItem{}, nil
	}
	rows, err := s.q.Ctx(ctx).ListInboxItemsByFeed(ctx, queries.ListInboxItemsByFeedParams{ProfileID: profileID, FeedID: feedID, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("listing inbox items for feed %q: %w", feedID, err)
	}
	return s.mapper.Slice(rows), nil
}

func (s *InboxItemStore) ListArchivedByFeed(ctx context.Context, profileID, feedID string, limit int) ([]InboxItem, error) {
	if limit <= 0 {
		return []InboxItem{}, nil
	}
	rows, err := s.q.Ctx(ctx).ListArchivedInboxItemsByFeed(ctx, queries.ListArchivedInboxItemsByFeedParams{ProfileID: profileID, FeedID: feedID, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("listing archived inbox items for feed %q: %w", feedID, err)
	}
	return s.mapper.Slice(rows), nil
}

// Trash includes unrouted and ignored items and has no unread semantics.
func (s *InboxItemStore) ListTrash(ctx context.Context, profileID string, limit int) ([]InboxItem, error) {
	if limit <= 0 {
		return []InboxItem{}, nil
	}
	rows, err := s.q.Ctx(ctx).ListInboxItemsTrash(ctx, queries.ListInboxItemsTrashParams{ProfileID: profileID, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("listing trash inbox items for %q: %w", profileID, err)
	}
	return s.mapper.Slice(rows), nil
}

// An empty profileID matches all profiles; feed membership and triage state
// are not filtered.
func (s *InboxItemStore) ListAll(ctx context.Context, profileID string, limit int) ([]InboxItem, error) {
	if limit <= 0 {
		return []InboxItem{}, nil
	}
	rows, err := s.q.Ctx(ctx).ListAllInboxItems(ctx, queries.ListAllInboxItemsParams{ProfileID: profileID, Lim: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("listing all inbox items: %w", err)
	}
	return s.mapper.Slice(rows), nil
}

// Archived memberships remain frozen and are excluded from synthetic replay.
func (s *InboxItemStore) ListUnarchived(ctx context.Context, profileID string) ([]InboxItem, error) {
	rows, err := s.q.Ctx(ctx).ListUnarchivedInboxItemsByProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("listing unarchived inbox items for %q: %w", profileID, err)
	}
	return s.mapper.Slice(rows), nil
}

func (s *InboxItemStore) ListUnarchivedBySource(ctx context.Context, profileID, sourceKind, sourceScope string) ([]InboxItem, error) {
	rows, err := s.q.Ctx(ctx).ListUnarchivedInboxItemsBySource(ctx, queries.ListUnarchivedInboxItemsBySourceParams{
		ProfileID: profileID, SourceKind: sourceKind, SourceScope: sourceScope,
	})
	if err != nil {
		return nil, fmt.Errorf("listing unarchived inbox items for source %s/%s: %w", sourceKind, sourceScope, err)
	}
	return s.mapper.Slice(rows), nil
}

// externalID is not unique across profiles or source scopes; an empty
// profileID matches all profiles.
func (s *InboxItemStore) FindByExternalID(ctx context.Context, profileID, externalID string) ([]InboxItem, error) {
	rows, err := s.q.Ctx(ctx).FindInboxItemsByExternalID(ctx, queries.FindInboxItemsByExternalIDParams{
		ExternalID: externalID, ProfileID: profileID,
	})
	if err != nil {
		return nil, fmt.Errorf("finding inbox items for external id %q: %w", externalID, err)
	}
	return s.mapper.Slice(rows), nil
}

func (s *InboxItemStore) GetByID(ctx context.Context, id int64) (InboxItem, error) {
	row, err := s.q.Ctx(ctx).GetInboxItemByID(ctx, id)
	return s.mapper.Err(row, errTransformQueryOne("inbox_item", fmt.Sprint(id), err))
}

func (s *InboxItemStore) RefByID(ctx context.Context, itemID int64) (models.ItemRef, error) {
	row, err := s.q.Ctx(ctx).GetInboxItemByID(ctx, itemID)
	if err != nil {
		return models.ItemRef{}, errTransformQueryOne("inbox_item", fmt.Sprint(itemID), err)
	}
	return models.ItemRef{
		ProfileID:   row.ProfileID,
		SourceKind:  row.SourceKind,
		SourceScope: row.SourceScope,
		ExternalID:  row.ExternalID,
	}, nil
}

// On a scoped miss, ResolveScoped migrates a legacy empty-scope row and its
// session links so later ingest cannot create a duplicate. A genuine miss
// returns sql.ErrNoRows.
func (s *InboxItemStore) ResolveScoped(ctx context.Context, profileID, sourceKind, sourceScope, externalID string) (InboxItem, error) {
	item, err := s.q.Ctx(ctx).GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
		ProfileID: profileID, SourceKind: sourceKind, SourceScope: sourceScope, ExternalID: externalID,
	})
	if err == nil {
		return s.mapper(item), nil
	}
	if !errors.Is(err, sql.ErrNoRows) || sourceScope == "" {
		return InboxItem{}, err
	}

	var legacy queries.InboxItem
	txErr := s.q.WithinTx(ctx, func(ctx context.Context, q *queries.DB) error {
		var legacyErr error
		legacy, legacyErr = q.GetInboxItemByExternalID(ctx, queries.GetInboxItemByExternalIDParams{
			ProfileID: profileID, SourceKind: sourceKind, SourceScope: "", ExternalID: externalID,
		})
		if legacyErr != nil {
			if errors.Is(legacyErr, sql.ErrNoRows) {
				return err
			}
			return legacyErr
		}
		if rescopeErr := q.RescopeInboxItem(ctx, queries.RescopeInboxItemParams{SourceScope: sourceScope, ID: legacy.ID}); rescopeErr != nil {
			return rescopeErr
		}
		// item_session is keyed on these same coordinates, so the links have
		// to move with the row or they address a scope nothing reads under
		// again.
		return s.sessions.Rescope(ctx, profileID, sourceKind, externalID, sourceScope)
	})
	if txErr != nil {
		return InboxItem{}, txErr
	}
	legacy.SourceScope = sourceScope
	return s.mapper(legacy), nil
}

// ResolveScopedID is ResolveScoped for a caller that needs only the row id, so
// a commit does not read the payload of every item its outputs name.
func (s *InboxItemStore) ResolveScopedID(ctx context.Context, profileID, sourceKind, sourceScope, externalID string) (int64, error) {
	id, err := s.q.Ctx(ctx).GetInboxItemIDByExternalID(ctx, queries.GetInboxItemIDByExternalIDParams{
		ProfileID: profileID, SourceKind: sourceKind, SourceScope: sourceScope, ExternalID: externalID,
	})
	if !errors.Is(err, sql.ErrNoRows) || sourceScope == "" {
		return id, err
	}
	item, err := s.ResolveScoped(ctx, profileID, sourceKind, sourceScope, externalID)
	return item.ID, err
}

// A missing identity returns (0, nil), because synthesized outputs may have
// no inbox row to link.
func (s *InboxItemStore) IDByExternalID(ctx context.Context, profileID, sourceKind, sourceScope, externalID string) (int64, error) {
	if externalID == "" {
		return 0, nil
	}
	id, err := s.q.Ctx(ctx).GetInboxItemIDByExternalID(ctx, queries.GetInboxItemIDByExternalIDParams{
		ProfileID: profileID, SourceKind: sourceKind, SourceScope: sourceScope, ExternalID: externalID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolving inbox item %s/%s/%s: %w", sourceKind, sourceScope, externalID, err)
	}
	return id, nil
}

// Unclaimed items return "". Multiple claims resolve to the lowest feed ID
// for a stable result.
func (s *InboxItemStore) FeedIDForItem(ctx context.Context, profileID string, itemID int64) (string, error) {
	feedID, err := s.q.Ctx(ctx).GetFeedIDForItem(ctx, queries.GetFeedIDForItemParams{ProfileID: profileID, ItemID: itemID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolving feed for inbox item %d: %w", itemID, err)
	}
	return feedID, nil
}

// Results use the lowest feed ID per item. Unclaimed IDs are absent from the
// map.
func (s *InboxItemStore) FeedIDsForItems(ctx context.Context, itemIDs []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}
	rows, err := s.q.Ctx(ctx).ListFeedIDsForItems(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("resolving feeds for %d inbox items: %w", len(itemIDs), err)
	}
	for _, row := range rows {
		out[row.ItemID] = row.FeedID
	}
	return out, nil
}

func (s *InboxItemStore) Events(ctx context.Context, itemID int64, limit int) ([]InboxEvent, error) {
	if limit <= 0 {
		return []InboxEvent{}, nil
	}
	rows, err := s.q.Ctx(ctx).ListInboxEventsByItem(ctx, queries.ListInboxEventsByItemParams{ItemID: itemID, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("listing inbox events for %d: %w", itemID, err)
	}
	events := make([]InboxEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, mapInboxEventFromDB(row))
	}
	return events, nil
}

// A revision mismatch returns ErrStale, not NotFoundError.
func (s *InboxItemStore) SetUnread(ctx context.Context, itemID, revision int64, unread bool) (InboxItem, error) {
	row, err := s.q.Ctx(ctx).SetInboxItemUnread(ctx, queries.SetInboxItemUnreadParams{Unread: boolToInt64(unread), ID: itemID, Revision: revision})
	if errors.Is(err, sql.ErrNoRows) {
		return InboxItem{}, ErrStale
	}
	if err != nil {
		return InboxItem{}, fmt.Errorf("setting inbox item %d unread: %w", itemID, err)
	}
	return s.mapper(row), nil
}

// An empty feedID clears the whole profile. The return value is the number of
// changed rows.
func (s *InboxItemStore) MarkRead(ctx context.Context, profileID, feedID string) (int64, error) {
	if profileID == "" {
		return 0, fmt.Errorf("marking inbox items read: profile id is required")
	}
	q := s.q.Ctx(ctx)
	if feedID == "" {
		marked, err := q.MarkProfileInboxItemsRead(ctx, profileID)
		if err != nil {
			return 0, fmt.Errorf("marking every feed read for %q: %w", profileID, err)
		}
		return marked, nil
	}
	marked, err := q.MarkFeedInboxItemsRead(ctx, queries.MarkFeedInboxItemsReadParams{ProfileID: profileID, FeedID: feedID})
	if err != nil {
		return 0, fmt.Errorf("marking feed %q read: %w", feedID, err)
	}
	return marked, nil
}

// Archiving uses the store clock; a revision mismatch returns ErrStale.
func (s *InboxItemStore) ToggleArchived(ctx context.Context, itemID, revision int64) (InboxItem, error) {
	row, err := s.q.Ctx(ctx).ToggleInboxItemArchived(ctx, queries.ToggleInboxItemArchivedParams{
		ArchivedAt: sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}, ID: itemID, Revision: revision,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return InboxItem{}, ErrStale
	}
	if err != nil {
		return InboxItem{}, fmt.Errorf("toggling archive for inbox item %d: %w", itemID, err)
	}
	return s.mapper(row), nil
}

// Ignoring uses the store clock; a revision mismatch returns ErrStale.
func (s *InboxItemStore) ToggleIgnored(ctx context.Context, itemID, revision int64) (InboxItem, error) {
	row, err := s.q.Ctx(ctx).ToggleInboxItemIgnored(ctx, queries.ToggleInboxItemIgnoredParams{
		IgnoredAt: sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}, ID: itemID, Revision: revision,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return InboxItem{}, ErrStale
	}
	if err != nil {
		return InboxItem{}, fmt.Errorf("toggling ignored state for inbox item %d: %w", itemID, err)
	}
	return s.mapper(row), nil
}

func (s *InboxItemStore) FeedCounts(ctx context.Context, profileID string) ([]FeedCount, error) {
	rows, err := s.q.Ctx(ctx).CountInboxItemsByFeed(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("counting inbox items by feed for %q: %w", profileID, err)
	}
	counts := make([]FeedCount, 0, len(rows))
	for _, row := range rows {
		counts = append(counts, FeedCount(row))
	}
	return counts, nil
}

func (s *InboxItemStore) DeleteByProfile(ctx context.Context, profileID string) error {
	return wrap("deleting inbox items by profile", s.q.Ctx(ctx).DeleteInboxItemsByProfile(ctx, profileID))
}

func (s *InboxItemStore) GetUnarchivedByID(ctx context.Context, itemID int64, profileID string) (InboxItem, error) {
	row, err := s.q.Ctx(ctx).GetUnarchivedInboxItemByID(ctx, queries.GetUnarchivedInboxItemByIDParams{ID: itemID, ProfileID: profileID})
	if err != nil {
		return InboxItem{}, errTransformQueryOne("inbox_item", fmt.Sprint(itemID), err)
	}
	return s.mapper(row), nil
}

// CreateSynthesized persists feed outputs that did not pass through ingest.
// It derives presentation from the payload and marks them active; later
// snapshots remove only claims, while later ingest upserts the same identity.
//
// The payload is read exactly as the ingest boundary reads a source's own, so
// an author minting per-entity items stamps them with `updatedAt` the same way
// a connector does. A payload that omits it is stamped now: a feed row shows
// its age, and an item minted at the epoch reads as decades old.
func (s *InboxItemStore) CreateSynthesized(ctx context.Context, in InboxItemSynthesize) (InboxItem, error) {
	title, url, updatedAt := models.FeedFields(in.Payload)
	if title == "" {
		title = in.ExternalID
	}
	if updatedAt == 0 {
		updatedAt = in.Now
	}
	row, err := s.q.Ctx(ctx).InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: in.ProfileID, SourceKind: in.SourceKind, SourceScope: in.SourceScope, ExternalID: in.ExternalID,
		Title: title, Url: url, Payload: in.Payload, Unread: 1, Lifecycle: models.LifecycleActive.String(),
		FirstSeenAt: in.Now, LastEventAt: updatedAt,
	})
	if err != nil {
		return InboxItem{}, wrap("minting synthesized inbox item", err)
	}
	return s.mapper(row), nil
}

type ItemTriageState struct {
	Unread         bool
	ArchivedAt     *int64
	ArchivedActor  string
	ArchivedReason string
}

// Terminal transitions create system archives. Reopened system archives
// always resurface; manual archives follow the profile policy.
func applyTransition(prev ItemTriageState, c models.Classification, policy models.ResurfacePolicy) ItemTriageState {
	next := prev
	if c.Transition == models.TransitionEnteredTerminal {
		// A user archive wins a concurrent terminal observation. The item is
		// already hidden; replacing its actor with system would erase the
		// manual decision the in-transaction read deliberately observed.
		if prev.ArchivedActor == models.ArchivedActorManual.String() && prev.ArchivedAt != nil {
			return next
		}
		now := time.Now().UnixMilli()
		next.ArchivedAt = &now
		next.ArchivedActor = models.ArchivedActorSystem.String()
		next.Unread = false
		return next
	}
	if prev.ArchivedAt == nil {
		if c.Attention == models.AttentionActivity || c.Transition == models.TransitionLeftTerminal {
			next.Unread = true
		}
		return next
	}
	resurface := c.Transition == models.TransitionLeftTerminal ||
		(prev.ArchivedActor == models.ArchivedActorManual.String() && policy == models.ResurfacePolicyAll && c.Attention == models.AttentionActivity)
	if prev.ArchivedActor == models.ArchivedActorSystem.String() && c.Transition == models.TransitionLeftTerminal {
		resurface = true
	}
	if prev.ArchivedActor == models.ArchivedActorManual.String() && policy == models.ResurfacePolicyNever {
		resurface = false
	}
	if resurface {
		next.ArchivedAt = nil
		next.ArchivedActor = ""
		next.Unread = true
	}
	return next
}

// Preserve an existing archive reason. Legacy manual archives use "manual";
// new system archives use the classifier reason.
func archivedReason(prev, next ItemTriageState, c models.Classification) string {
	if next.ArchivedAt == nil {
		return ""
	}
	if prev.ArchivedAt != nil {
		if prev.ArchivedReason != "" {
			return prev.ArchivedReason
		}
		if next.ArchivedActor == models.ArchivedActorManual.String() {
			return models.ArchivedActorManual.String()
		}
		return ""
	}
	if next.ArchivedActor == models.ArchivedActorManual.String() {
		return models.ArchivedActorManual.String()
	}
	return c.ArchivedReason
}

// IngestObservation compares, classifies, persists, logs, and advances the
// source head in one transaction.
func (s *InboxItemStore) IngestObservation(ctx context.Context, classifier models.Classifier, p IngestObservationParams) (result IngestResult, err error) {
	if classifier == nil {
		return result, fmt.Errorf("ingesting observation: nil classifier")
	}
	if p.Current.ExternalID == "" {
		return result, fmt.Errorf("ingesting observation: external id is required")
	}
	if p.Policy == "" {
		p.Policy = models.ResurfacePolicyStateChanges
	}
	err = s.q.WithinTx(ctx, func(ctx context.Context, _ *queries.DB) error {
		head, headErr := s.heads.Payload(ctx, p.Topic, p.Current.ExternalID)
		if headErr == nil && bytes.Equal(head, p.Current.Payload) {
			return nil
		}
		if headErr != nil && !errors.Is(headErr, sql.ErrNoRows) {
			return fmt.Errorf("reading source head: %w", headErr)
		}

		var previous *models.Observation
		prevRow, getErr := s.ResolveScoped(ctx, p.ProfileID, p.Current.SourceKind, p.Current.SourceScope, p.Current.ExternalID)
		if getErr == nil {
			previous = &models.Observation{ExternalID: prevRow.ExternalID, Title: prevRow.Title, URL: prevRow.URL, SourceKind: prevRow.SourceKind, SourceScope: prevRow.SourceScope, ObservedAt: prevRow.LastEventAt, Payload: prevRow.Payload}
		} else if !errors.Is(getErr, sql.ErrNoRows) {
			return fmt.Errorf("reading inbox item: %w", getErr)
		}

		classification := classifier.Classify(previous, p.Current)
		if classification.Transition == "" {
			classification.Transition = models.TransitionNone
		}
		if classification.Attention == "" {
			classification.Attention = models.AttentionTrivial
		}
		if classification.Lifecycle == "" {
			classification.Lifecycle = models.LifecycleUnknown
		}
		classification.Detail = boundEventDetail(classification.Detail)
		prevTriage := ItemTriageState{}
		if getErr == nil {
			prevTriage.Unread = prevRow.Unread
			prevTriage.ArchivedAt = prevRow.ArchivedAt
			prevTriage.ArchivedActor = prevRow.ArchivedActor
			prevTriage.ArchivedReason = prevRow.ArchivedReason
		}
		triage := applyTransition(prevTriage, classification, p.Policy)
		archiveReason := archivedReason(prevTriage, triage, classification)
		now := s.now().UnixMilli()
		var archivedAt sql.NullInt64
		if triage.ArchivedAt != nil {
			archivedAt = sql.NullInt64{Int64: *triage.ArchivedAt, Valid: true}
		}
		item, upsertErr := s.q.Ctx(ctx).UpsertInboxItem(ctx, queries.UpsertInboxItemParams{
			ProfileID: p.ProfileID, SourceKind: p.Current.SourceKind, SourceScope: p.Current.SourceScope, ExternalID: p.Current.ExternalID,
			Title: p.Current.Title, Url: p.Current.URL, Payload: p.Current.Payload, Unread: boolToInt64(triage.Unread),
			ArchivedAt: archivedAt, ArchivedActor: null(triage.ArchivedActor), ArchivedReason: null(archiveReason),
			Lifecycle: classification.Lifecycle.String(), SourceState: null(classification.SourceState), FirstSeenAt: now, LastEventAt: p.Current.ObservedAt,
		})
		if upsertErr != nil {
			return fmt.Errorf("upserting inbox item: %w", upsertErr)
		}

		if classification.Transition != models.TransitionNone || classification.Attention != models.AttentionTrivial {
			var occurrence sql.NullString
			if classification.OccurrenceKey != "" {
				occurrence = sql.NullString{String: classification.OccurrenceKey, Valid: true}
			}
			_, eventErr := s.q.Ctx(ctx).InsertInboxEvent(ctx, queries.InsertInboxEventParams{ItemID: item.ID, Kind: classification.Kind, Transition: classification.Transition.String(), Attention: classification.Attention.String(), OccurrenceKey: occurrence, Summary: null(classification.Summary), Detail: classification.Detail, CreatedAt: now})
			if eventErr != nil && !errors.Is(eventErr, sql.ErrNoRows) {
				return fmt.Errorf("inserting inbox event: %w", eventErr)
			}
		}

		occurrence := classification.OccurrenceKey
		offset, appendErr := s.log.AppendObservation(ctx, p.Topic, p.Current.ExternalID, p.Current.Payload, p.Current.SourceKind, p.Current.SourceScope, occurrence, now)
		if appendErr != nil {
			return fmt.Errorf("appending event log: %w", appendErr)
		}
		if occurrence == "" {
			occurrence = fmt.Sprintf("%d", offset)
			if err := s.log.BackfillOccurrenceKey(ctx, offset, occurrence); err != nil {
				return fmt.Errorf("backfilling occurrence key: %w", err)
			}
		}
		if err := s.heads.Upsert(ctx, p.Topic, p.Current.ExternalID, p.Current.Payload); err != nil {
			return fmt.Errorf("updating source head: %w", err)
		}
		result = IngestResult{ItemID: item.ID, Revision: item.Revision, Classification: classification, Wrote: true, Offset: offset}
		return nil
	})
	if err != nil {
		return IngestResult{}, fmt.Errorf("ingesting observation: %w", err)
	}
	return result, nil
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
