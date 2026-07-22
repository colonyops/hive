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
	ID             int64
	ProfileID      string
	SourceKind     string
	SourceScope    string
	ExternalID     string
	Title          string
	URL            string
	Payload        json.RawMessage
	Revision       int64
	Unread         bool
	ArchivedAt     *int64
	ArchivedActor  string
	ArchivedReason string
	Lifecycle      string
	SourceState    string
	FirstSeenAt    int64
	LastEventAt    int64
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
