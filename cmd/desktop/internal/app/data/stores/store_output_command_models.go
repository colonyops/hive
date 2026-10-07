package stores

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type OutputCommand struct {
	ID           int64
	ActionID     string
	Key          string
	Payload      []byte
	Status       string
	Attempts     int64
	LastError    string
	ResultJSON   string
	Stdout       string
	Stderr       string
	CreatedAt    int64
	IsRerun      bool
	ProfileID    string
	SourceKind   string
	SourceScope  string
	ExternalID   string
	DispatchLane string
	ClaimToken   string
	ClaimedAt    int64
	NotBefore    int64
}

// Commands without an inbox origin return a zero ItemRef, for which Known is
// false.
func (c OutputCommand) ItemRef() models.ItemRef {
	return models.ItemRef{ProfileID: c.ProfileID, SourceKind: c.SourceKind, SourceScope: c.SourceScope, ExternalID: c.ExternalID}
}

func mapOutputCommandFromDB(row queries.OutputCommand) OutputCommand {
	return OutputCommand{
		ID: row.ID, ActionID: row.ActionID, Key: row.Key, Payload: row.Payload,
		Status: row.Status, Attempts: row.Attempts, LastError: row.LastError.String,
		ResultJSON: row.ResultJson.String, Stdout: row.Stdout.String, Stderr: row.Stderr.String,
		CreatedAt: row.CreatedAt, IsRerun: row.IsRerun != 0,
		ProfileID: row.ProfileID, SourceKind: row.SourceKind, SourceScope: row.SourceScope, ExternalID: row.ExternalID,
		DispatchLane: row.DispatchLane, ClaimToken: row.ClaimToken, ClaimedAt: row.ClaimedAt, NotBefore: row.NotBefore,
	}
}
