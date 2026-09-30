package stores

import (
	"encoding/json"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type InboxItem struct {
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
	ArchivedActor  string          `json:"archivedActor,omitempty"  jsonschema:"enum=manual,enum=system,description=Who archived the item; present only when archived."`
	ArchivedReason string          `json:"archivedReason,omitempty"`
	Lifecycle      string          `json:"lifecycle"                jsonschema:"enum=active,enum=terminal,enum=unknown,description=Whether the underlying item is still live (active) or resolved (terminal)."`
	SourceState    string          `json:"sourceState,omitempty"`
	FirstSeenAt    int64           `json:"firstSeenAt"`
	LastEventAt    int64           `json:"lastEventAt"`
	IgnoredAt      *int64          `json:"ignoredAt,omitempty"`
}

type InboxEvent struct {
	ID         int64           `json:"id"`
	ItemID     int64           `json:"itemId"`
	Kind       string          `json:"kind"`
	Transition string          `json:"transition"        jsonschema:"enum=none,enum=entered-terminal,enum=left-terminal,description=Lifecycle transition this event marks."`
	Attention  string          `json:"attention"         jsonschema:"enum=activity,enum=trivial,description=Whether the event is worth surfacing (activity) or routine (trivial)."`
	Summary    string          `json:"summary,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	CreatedAt  int64           `json:"createdAt"`
}

type FeedCount struct {
	FeedID   string `json:"feedId"`
	Total    int64  `json:"total"`
	Unread   int64  `json:"unread"`
	Archived int64  `json:"archived"`
}

type IngestObservationParams struct {
	ProfileID string
	Topic     string
	Policy    models.ResurfacePolicy
	Current   models.Observation
}

// Duplicate source-head payloads return the zero value with Wrote false.
type IngestResult struct {
	ItemID         int64
	Revision       int64
	Classification models.Classification
	Wrote          bool
	Offset         int64
}

type InboxItemSynthesize struct {
	ProfileID   string
	SourceKind  string
	SourceScope string
	ExternalID  string
	Payload     []byte
	// Now is shared across every synthesized item, node run, and KV write in one
	// commit.
	Now int64
}

const maxEventDetailBytes = 4096

func boundEventDetail(detail []byte) []byte {
	if len(detail) <= maxEventDetailBytes {
		return detail
	}
	return append([]byte(nil), detail[:maxEventDetailBytes]...)
}

func mapInboxItemFromDB(row queries.InboxItem) InboxItem {
	item := InboxItem{
		ID: row.ID, ProfileID: row.ProfileID, SourceKind: row.SourceKind,
		SourceScope: row.SourceScope, ExternalID: row.ExternalID, Title: row.Title,
		URL: row.Url, Payload: json.RawMessage(row.Payload), Revision: row.Revision,
		Unread: row.Unread != 0, Lifecycle: row.Lifecycle, FirstSeenAt: row.FirstSeenAt,
		LastEventAt: row.LastEventAt,
	}
	if row.ArchivedAt.Valid {
		archivedAt := row.ArchivedAt.Int64
		item.ArchivedAt = &archivedAt
	}
	if row.ArchivedActor.Valid {
		item.ArchivedActor = row.ArchivedActor.String
	}
	if row.ArchivedReason.Valid {
		item.ArchivedReason = row.ArchivedReason.String
	}
	if row.SourceState.Valid {
		item.SourceState = row.SourceState.String
	}
	if row.IgnoredAt.Valid {
		ignoredAt := row.IgnoredAt.Int64
		item.IgnoredAt = &ignoredAt
	}
	return item
}

func mapInboxEventFromDB(row queries.InboxEvent) InboxEvent {
	event := InboxEvent{ID: row.ID, ItemID: row.ItemID, Kind: row.Kind, Transition: row.Transition, Attention: row.Attention, CreatedAt: row.CreatedAt}
	if row.Summary.Valid {
		event.Summary = row.Summary.String
	}
	if len(row.Detail) > 0 {
		event.Detail = json.RawMessage(row.Detail)
	}
	return event
}
