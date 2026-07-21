package pipelinedb

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

// JobRecord is the storage shape of one job row. Status and step stay plain
// strings at this layer; the jobs package owns the typed status and converts at
// its boundary. CommandID is nil until the job is linked to an output_command.
type JobRecord struct {
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

// InsertJob persists one job and returns the stored row with its assigned id.
func (db *DB) InsertJob(ctx context.Context, rec JobRecord) (JobRecord, error) {
	row, err := db.queries.InsertJob(ctx, InsertJobParams{
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
		Status:    rec.Status,
		Label:     rec.Label,
		Step:      rec.Step,
		ActionID:  rec.ActionID,
		Target:    rec.Target,
		Error:     rec.Error,
		CommandID: nullableInt64(rec.CommandID),
	})
	if err != nil {
		return JobRecord{}, fmt.Errorf("inserting job %q: %w", rec.Label, err)
	}
	return jobRecordFromRow(row), nil
}

// SetJobRunning advances a job to running and links its output_command. This is
// the only job update that writes command_id.
func (db *DB) SetJobRunning(
	ctx context.Context,
	id int64,
	updatedAt int64,
	step string,
	commandID int64,
) (JobRecord, error) {
	row, err := db.queries.SetJobRunning(ctx, SetJobRunningParams{
		UpdatedAt: updatedAt,
		Status:    "running",
		Step:      step,
		CommandID: sql.NullInt64{Int64: commandID, Valid: true},
		ID:        id,
	})
	if err != nil {
		return JobRecord{}, fmt.Errorf("setting job %d running: %w", id, err)
	}
	return jobRecordFromRow(row), nil
}

// SetJobStatus advances a job's status, step, and error without changing its
// command_id link.
func (db *DB) SetJobStatus(
	ctx context.Context,
	id int64,
	updatedAt int64,
	status string,
	step string,
	errText string,
) (JobRecord, error) {
	row, err := db.queries.SetJobStatus(ctx, SetJobStatusParams{
		UpdatedAt: updatedAt,
		Status:    status,
		Step:      step,
		Error:     errText,
		ID:        id,
	})
	if err != nil {
		return JobRecord{}, fmt.Errorf("setting job %d status to %q: %w", id, status, err)
	}
	return jobRecordFromRow(row), nil
}

// FindRunningJobByCommandID returns the running job linked to commandID. The
// boolean is false when no such job exists.
func (db *DB) FindRunningJobByCommandID(ctx context.Context, commandID int64) (JobRecord, bool, error) {
	row, err := db.queries.FindRunningJobByCommandID(ctx, sql.NullInt64{Int64: commandID, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return JobRecord{}, false, nil
		}
		return JobRecord{}, false, fmt.Errorf("finding running job for command %d: %w", commandID, err)
	}
	return jobRecordFromRow(row), true, nil
}

// ListJobs returns up to limit jobs with id < before, newest first. Pass before
// <= 0 to start from the most recent job.
func (db *DB) ListJobs(ctx context.Context, before int64, limit int) ([]JobRecord, error) {
	if before <= 0 {
		before = math.MaxInt64
	}
	rows, err := db.queries.ListJobs(ctx, ListJobsParams{
		ID:    before,
		Limit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}
	return jobRecordsFromRows(rows), nil
}

// ListActiveJobs returns non-terminal jobs and terminal jobs updated at or
// after since, newest first.
func (db *DB) ListActiveJobs(ctx context.Context, since int64) ([]JobRecord, error) {
	rows, err := db.queries.ListActiveJobs(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("listing active jobs: %w", err)
	}
	return jobRecordsFromRows(rows), nil
}

func jobRecordsFromRows(rows []Job) []JobRecord {
	out := make([]JobRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, jobRecordFromRow(row))
	}
	return out
}

func jobRecordFromRow(row Job) JobRecord {
	var commandID *int64
	if row.CommandID.Valid {
		value := row.CommandID.Int64
		commandID = &value
	}
	return JobRecord{
		ID:        row.ID,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		Status:    row.Status,
		Label:     row.Label,
		Step:      row.Step,
		ActionID:  row.ActionID,
		Target:    row.Target,
		Error:     row.Error,
		CommandID: commandID,
	}
}

func nullableInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}
