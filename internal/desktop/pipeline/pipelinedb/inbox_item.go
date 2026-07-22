package pipelinedb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// InboxItemView is the read-side shape used by inbox callers.
type InboxItemView struct {
	ID             int64           `json:"id"`
	ProfileID      string          `json:"profileId"`
	SourceKind     string          `json:"sourceKind"`
	SourceScope    string          `json:"sourceScope"`
	ExternalID     string          `json:"externalId"`
	Title          string          `json:"title"`
	URL            string          `json:"url"`
	Payload        json.RawMessage `json:"payload"`
	Revision       int64           `json:"revision"`
	Unread         bool            `json:"unread"`
	ArchivedAt     *int64          `json:"archivedAt,omitempty"`
	ArchivedActor  string          `json:"archivedActor,omitempty"`
	ArchivedReason string          `json:"archivedReason,omitempty"`
	Lifecycle      string          `json:"lifecycle"`
	SourceState    string          `json:"sourceState,omitempty"`
	FirstSeenAt    int64           `json:"firstSeenAt"`
	LastEventAt    int64           `json:"lastEventAt"`
	IgnoredAt      *int64          `json:"ignoredAt,omitempty"`
}

type InboxEventView struct {
	ID         int64           `json:"id"`
	ItemID     int64           `json:"itemId"`
	Kind       string          `json:"kind"`
	Transition string          `json:"transition"`
	Attention  string          `json:"attention"`
	Summary    string          `json:"summary,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	CreatedAt  int64           `json:"createdAt"`
}

type InboxCounts struct {
	InboxTotal  int64 `json:"inboxTotal"`
	InboxUnread int64 `json:"inboxUnread"`
}

type FeedInboxCount struct {
	FeedID string `json:"feedId"`
	Total  int64  `json:"total"`
	Unread int64  `json:"unread"`
}

// ErrStaleInboxItem indicates the item changed after the caller read its
// revision. Callers should re-read rather than applying an optimistic result.
var ErrStaleInboxItem = errors.New("stale inbox item revision")

func inboxItemView(row InboxItem) InboxItemView {
	view := InboxItemView{
		ID: row.ID, ProfileID: row.ProfileID, SourceKind: row.SourceKind,
		SourceScope: row.SourceScope, ExternalID: row.ExternalID, Title: row.Title,
		URL: row.Url, Payload: json.RawMessage(row.Payload), Revision: row.Revision,
		Unread: row.Unread != 0, Lifecycle: row.Lifecycle, FirstSeenAt: row.FirstSeenAt,
		LastEventAt: row.LastEventAt,
	}
	if row.ArchivedAt.Valid {
		archivedAt := row.ArchivedAt.Int64
		view.ArchivedAt = &archivedAt
	}
	if row.ArchivedActor.Valid {
		view.ArchivedActor = row.ArchivedActor.String
	}
	if row.ArchivedReason.Valid {
		view.ArchivedReason = row.ArchivedReason.String
	}
	if row.SourceState.Valid {
		view.SourceState = row.SourceState.String
	}
	if row.IgnoredAt.Valid {
		ignoredAt := row.IgnoredAt.Int64
		view.IgnoredAt = &ignoredAt
	}
	return view
}

func inboxItemViews(rows []InboxItem) []InboxItemView {
	views := make([]InboxItemView, 0, len(rows))
	for _, row := range rows {
		views = append(views, inboxItemView(row))
	}
	return views
}

func (db *DB) ListInboxItems(ctx context.Context, profileID, view string, limit int) ([]InboxItemView, error) {
	if limit <= 0 {
		return []InboxItemView{}, nil
	}
	var rows []InboxItem
	var err error
	switch view {
	case "inbox":
		rows, err = db.queries.ListInboxItemsInbox(ctx, ListInboxItemsInboxParams{ProfileID: profileID, Limit: int64(limit)})
	case "open":
		rows, err = db.queries.ListInboxItemsOpen(ctx, ListInboxItemsOpenParams{ProfileID: profileID, Limit: int64(limit)})
	case "archive":
		rows, err = db.queries.ListInboxItemsArchive(ctx, ListInboxItemsArchiveParams{ProfileID: profileID, Limit: int64(limit)})
	case "all":
		rows, err = db.queries.ListInboxItemsAll(ctx, ListInboxItemsAllParams{ProfileID: profileID, Limit: int64(limit)})
	case "ignored":
		rows, err = db.queries.ListInboxItemsIgnored(ctx, ListInboxItemsIgnoredParams{ProfileID: profileID, Limit: int64(limit)})
	default:
		return nil, fmt.Errorf("unknown inbox view %q", view)
	}
	if err != nil {
		return nil, fmt.Errorf("listing %s inbox items for %q: %w", view, profileID, err)
	}
	return inboxItemViews(rows), nil
}

func (db *DB) ListInboxItemsByFeed(ctx context.Context, profileID, feedID string, limit int) ([]InboxItemView, error) {
	if limit <= 0 {
		return []InboxItemView{}, nil
	}
	rows, err := db.queries.ListInboxItemsByFeed(ctx, ListInboxItemsByFeedParams{ProfileID: profileID, FeedID: feedID, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("listing inbox items for feed %q: %w", feedID, err)
	}
	return inboxItemViews(rows), nil
}

func (db *DB) InboxItemEvents(ctx context.Context, itemID int64, limit int) ([]InboxEventView, error) {
	if limit <= 0 {
		return []InboxEventView{}, nil
	}
	rows, err := db.queries.ListInboxEventsByItem(ctx, ListInboxEventsByItemParams{ItemID: itemID, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("listing inbox events for %d: %w", itemID, err)
	}
	views := make([]InboxEventView, 0, len(rows))
	for _, row := range rows {
		view := InboxEventView{ID: row.ID, ItemID: row.ItemID, Kind: row.Kind, Transition: row.Transition, Attention: row.Attention, CreatedAt: row.CreatedAt}
		if row.Summary.Valid {
			view.Summary = row.Summary.String
		}
		if len(row.Detail) > 0 {
			view.Detail = json.RawMessage(row.Detail)
		}
		views = append(views, view)
	}
	return views, nil
}

func (db *DB) SetInboxItemUnread(ctx context.Context, itemID, revision int64, unread bool) (InboxItemView, error) {
	row, err := db.queries.SetInboxItemUnread(ctx, SetInboxItemUnreadParams{Unread: boolInt(unread), ID: itemID, Revision: revision})
	if errors.Is(err, sql.ErrNoRows) {
		return InboxItemView{}, ErrStaleInboxItem
	}
	if err != nil {
		return InboxItemView{}, fmt.Errorf("setting inbox item %d unread: %w", itemID, err)
	}
	return inboxItemView(row), nil
}

func (db *DB) ToggleInboxItemArchived(ctx context.Context, itemID, revision, archivedAt int64) (InboxItemView, error) {
	row, err := db.queries.ToggleInboxItemArchived(ctx, ToggleInboxItemArchivedParams{ArchivedAt: sql.NullInt64{Int64: archivedAt, Valid: true}, ID: itemID, Revision: revision})
	if errors.Is(err, sql.ErrNoRows) {
		return InboxItemView{}, ErrStaleInboxItem
	}
	if err != nil {
		return InboxItemView{}, fmt.Errorf("toggling archive for inbox item %d: %w", itemID, err)
	}
	return inboxItemView(row), nil
}

func (db *DB) ToggleInboxItemIgnored(ctx context.Context, itemID, revision, ignoredAt int64) (InboxItemView, error) {
	row, err := db.queries.ToggleInboxItemIgnored(ctx, ToggleInboxItemIgnoredParams{IgnoredAt: sql.NullInt64{Int64: ignoredAt, Valid: true}, ID: itemID, Revision: revision})
	if errors.Is(err, sql.ErrNoRows) {
		return InboxItemView{}, ErrStaleInboxItem
	}
	if err != nil {
		return InboxItemView{}, fmt.Errorf("toggling ignored state for inbox item %d: %w", itemID, err)
	}
	return inboxItemView(row), nil
}

func (db *DB) InboxCounts(ctx context.Context, profileID string) (InboxCounts, error) {
	row, err := db.queries.CountInboxItems(ctx, profileID)
	if err != nil {
		return InboxCounts{}, fmt.Errorf("counting inbox items for %q: %w", profileID, err)
	}
	return InboxCounts(row), nil
}

func (db *DB) FeedCounts(ctx context.Context, profileID string) ([]FeedInboxCount, error) {
	rows, err := db.queries.CountInboxItemsByFeed(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("counting inbox items by feed for %q: %w", profileID, err)
	}
	counts := make([]FeedInboxCount, 0, len(rows))
	for _, row := range rows {
		counts = append(counts, FeedInboxCount(row))
	}
	return counts, nil
}

type ItemTriageState struct {
	Unread         bool
	ArchivedAt     *int64
	ArchivedActor  string
	ArchivedReason string
}

// applyTransition is intentionally SQL-free so archive semantics remain easy
// to test. Terminal transitions are system-owned; system archived items always
// return on a reopen, while manual archives obey the profile policy.
func applyTransition(prev ItemTriageState, c Classification, policy ResurfacePolicy) ItemTriageState {
	next := prev
	if c.Transition == TransitionEnteredTerminal {
		// A user archive wins a concurrent terminal observation. The item is
		// already hidden; replacing its actor with system would erase the
		// manual decision the in-transaction read deliberately observed.
		if prev.ArchivedActor == ArchivedActorManual.String() && prev.ArchivedAt != nil {
			return next
		}
		now := time.Now().UnixMilli()
		next.ArchivedAt = &now
		next.ArchivedActor = ArchivedActorSystem.String()
		next.Unread = false
		return next
	}
	if prev.ArchivedAt == nil {
		if c.Attention == AttentionActivity || c.Transition == TransitionLeftTerminal {
			next.Unread = true
		}
		return next
	}
	resurface := c.Transition == TransitionLeftTerminal ||
		(prev.ArchivedActor == ArchivedActorManual.String() && policy == ResurfacePolicyAll && c.Attention == AttentionActivity)
	if prev.ArchivedActor == ArchivedActorSystem.String() && c.Transition == TransitionLeftTerminal {
		resurface = true
	}
	if prev.ArchivedActor == ArchivedActorManual.String() && policy == ResurfacePolicyNever {
		resurface = false
	}
	if resurface {
		next.ArchivedAt = nil
		next.ArchivedActor = ""
		next.Unread = true
	}
	return next
}

// archivedReason retains the reason for an item that remains archived. A
// manual archive predating reason tracking is labeled manual; a newly system-
// archived terminal item takes the classifier's source-specific reason.
func archivedReason(prev, next ItemTriageState, c Classification) string {
	if next.ArchivedAt == nil {
		return ""
	}
	if prev.ArchivedAt != nil {
		if prev.ArchivedReason != "" {
			return prev.ArchivedReason
		}
		if next.ArchivedActor == ArchivedActorManual.String() {
			return ArchivedActorManual.String()
		}
		return ""
	}
	if next.ArchivedActor == ArchivedActorManual.String() {
		return ArchivedActorManual.String()
	}
	return c.ArchivedReason
}

type IngestObservationParams struct {
	ProfileID string
	Topic     string
	Policy    ResurfacePolicy
	Current   Observation
}

type IngestResult struct {
	ItemID         int64
	Revision       int64
	Classification Classification
	Wrote          bool
	Offset         int64
}

// IngestObservation is the persistence boundary for a source item. The source
// head comparison, classification, inbox mutation, event log append and source
// head update share one immediate SQLite transaction.
func (db *DB) IngestObservation(ctx context.Context, classifier Classifier, p IngestObservationParams) (result IngestResult, err error) {
	if classifier == nil {
		return result, fmt.Errorf("ingesting observation: nil classifier")
	}
	if p.Current.ExternalID == "" {
		return result, fmt.Errorf("ingesting observation: external id is required")
	}
	if p.Policy == "" {
		p.Policy = ResurfacePolicyStateChanges
	}
	err = db.WithTx(ctx, func(q *Queries) error {
		head, headErr := q.GetSourceHeadPayload(ctx, GetSourceHeadPayloadParams{Topic: p.Topic, Key: p.Current.ExternalID})
		if headErr == nil && bytes.Equal(head, p.Current.Payload) {
			return nil
		}
		if headErr != nil && !errors.Is(headErr, sql.ErrNoRows) {
			return fmt.Errorf("reading source head: %w", headErr)
		}

		var previous *Observation
		prevRow, getErr := q.GetInboxItemByExternalID(ctx, GetInboxItemByExternalIDParams{
			ProfileID: p.ProfileID, SourceKind: p.Current.SourceKind, SourceScope: p.Current.SourceScope, ExternalID: p.Current.ExternalID,
		})
		if getErr == nil {
			previous = &Observation{ExternalID: prevRow.ExternalID, Title: prevRow.Title, URL: prevRow.Url, SourceKind: prevRow.SourceKind, SourceScope: prevRow.SourceScope, ObservedAt: prevRow.LastEventAt, Payload: prevRow.Payload}
		} else if !errors.Is(getErr, sql.ErrNoRows) {
			return fmt.Errorf("reading inbox item: %w", getErr)
		}

		classification := classifier.Classify(previous, p.Current)
		if classification.Transition == "" {
			classification.Transition = TransitionNone
		}
		if classification.Attention == "" {
			classification.Attention = AttentionTrivial
		}
		if classification.Lifecycle == "" {
			classification.Lifecycle = LifecycleUnknown
		}
		classification.Detail = boundEventDetail(classification.Detail)
		prevTriage := ItemTriageState{}
		if getErr == nil {
			prevTriage.Unread = prevRow.Unread != 0
			if prevRow.ArchivedAt.Valid {
				v := prevRow.ArchivedAt.Int64
				prevTriage.ArchivedAt = &v
			}
			if prevRow.ArchivedActor.Valid {
				prevTriage.ArchivedActor = prevRow.ArchivedActor.String
			}
			if prevRow.ArchivedReason.Valid {
				prevTriage.ArchivedReason = prevRow.ArchivedReason.String
			}
		}
		triage := applyTransition(prevTriage, classification, p.Policy)
		archiveReason := archivedReason(prevTriage, triage, classification)
		now := time.Now().UnixMilli()
		var archivedAt sql.NullInt64
		if triage.ArchivedAt != nil {
			archivedAt = sql.NullInt64{Int64: *triage.ArchivedAt, Valid: true}
		}
		item, upsertErr := q.UpsertInboxItem(ctx, UpsertInboxItemParams{
			ProfileID: p.ProfileID, SourceKind: p.Current.SourceKind, SourceScope: p.Current.SourceScope, ExternalID: p.Current.ExternalID,
			Title: p.Current.Title, Url: p.Current.URL, Payload: p.Current.Payload, Unread: boolInt(triage.Unread),
			ArchivedAt: archivedAt, ArchivedActor: null(triage.ArchivedActor), ArchivedReason: null(archiveReason),
			Lifecycle: classification.Lifecycle.String(), SourceState: null(classification.SourceState), FirstSeenAt: now, LastEventAt: p.Current.ObservedAt,
		})
		if upsertErr != nil {
			return fmt.Errorf("upserting inbox item: %w", upsertErr)
		}

		if classification.Transition != TransitionNone || classification.Attention != AttentionTrivial {
			var occurrence sql.NullString
			if classification.OccurrenceKey != "" {
				occurrence = sql.NullString{String: classification.OccurrenceKey, Valid: true}
			}
			_, eventErr := q.InsertInboxEvent(ctx, InsertInboxEventParams{ItemID: item.ID, Kind: classification.Kind, Transition: classification.Transition.String(), Attention: classification.Attention.String(), OccurrenceKey: occurrence, Summary: null(classification.Summary), Detail: classification.Detail, CreatedAt: now})
			if eventErr != nil && !errors.Is(eventErr, sql.ErrNoRows) {
				return fmt.Errorf("inserting inbox event: %w", eventErr)
			}
		}

		occurrence := classification.OccurrenceKey
		offset, appendErr := q.AppendEvent(ctx, AppendEventParams{Topic: p.Topic, Key: p.Current.ExternalID, Payload: p.Current.Payload, Snapshot: 0, SourceKind: p.Current.SourceKind, SourceScope: p.Current.SourceScope, OccurrenceKey: null(occurrence), CreatedAt: now})
		if appendErr != nil {
			return fmt.Errorf("appending event log: %w", appendErr)
		}
		if occurrence == "" {
			occurrence = fmt.Sprintf("%d", offset)
			if err := q.UpdateEventOccurrenceKey(ctx, UpdateEventOccurrenceKeyParams{OccurrenceKey: sql.NullString{String: occurrence, Valid: true}, Offset: offset}); err != nil {
				return fmt.Errorf("backfilling occurrence key: %w", err)
			}
		}
		if err := q.UpsertSourceHead(ctx, UpsertSourceHeadParams{Topic: p.Topic, Key: p.Current.ExternalID, Payload: p.Current.Payload}); err != nil {
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

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
