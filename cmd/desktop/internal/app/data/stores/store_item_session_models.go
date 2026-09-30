package stores

import "github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"

// ItemSession is one recorded hive session created on an inbox item's
// behalf.
type ItemSession struct {
	SessionID   string `json:"sessionId"`
	ProfileID   string `json:"profileId"`
	SourceKind  string `json:"sourceKind"`
	SourceScope string `json:"sourceScope"`
	ExternalID  string `json:"externalId"`
	CreatedAt   int64  `json:"createdAt"`
}

func mapItemSessionFromDB(row queries.ItemSession) ItemSession {
	return ItemSession{
		SessionID:   row.SessionID,
		ProfileID:   row.ProfileID,
		SourceKind:  row.SourceKind,
		SourceScope: row.SourceScope,
		ExternalID:  row.ExternalID,
		CreatedAt:   row.CreatedAt,
	}
}
