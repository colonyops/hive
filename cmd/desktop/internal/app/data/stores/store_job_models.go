package stores

import "github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"

// Status and Step remain strings because the jobs package owns their typed
// mapping.
type Job struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Status    string `json:"status"`
	Label     string `json:"label"`
	Step      string `json:"step"`
	ActionID  string `json:"actionId"`
	Target    string `json:"target"`
	Error     string `json:"error"`
	CommandID *int64 `json:"commandId,omitempty"`
}

type JobCreate struct {
	Status   string
	Label    string
	Step     string
	ActionID string
	Target   string
	Error    string
}

func mapJobFromDB(row queries.Job) Job {
	var commandID *int64
	if row.CommandID.Valid {
		value := row.CommandID.Int64
		commandID = &value
	}
	return Job{
		ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Status: row.Status,
		Label: row.Label, Step: row.Step, ActionID: row.ActionID, Target: row.Target,
		Error: row.Error, CommandID: commandID,
	}
}
