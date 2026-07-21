// Package jobs is the desktop app's live action-run lifecycle. Unlike the
// activity audit log (terminal events only), a job is written at every
// transition (queued -> running -> done|failed) so the titlebar spinner chip
// can show in-flight work. A job links to its originating output_command via a
// nullable CommandID so the popover can deep-link to ActionRun detail.
package jobs

import "time"

// DefaultLingerWindow is how long terminal jobs remain in the live-jobs view.
// The backend owns this window so frontend clients do not need a matching
// timeout constant.
const DefaultLingerWindow = 4 * time.Second

// JobStatus is a job's lifecycle stage.
//
// ENUM(queued, running, done, failed)
type JobStatus string

// Job is one action-run lifecycle row. ID, CreatedAt, and UpdatedAt are
// assigned by the store. CreatedAt and UpdatedAt are unix milliseconds. Step
// is a human label derived from Status in v1.
type Job struct {
	ID        int64     `json:"id"`
	CreatedAt int64     `json:"createdAt"`
	UpdatedAt int64     `json:"updatedAt"`
	Status    JobStatus `json:"status"`
	Label     string    `json:"label"`
	Step      string    `json:"step"`
	ActionID  string    `json:"actionId"`
	Target    string    `json:"target"`
	Error     string    `json:"error,omitempty"`
	CommandID *int64    `json:"commandId,omitempty"`
}

// stepFor returns the human step label for a status transition.
func stepFor(status JobStatus) string {
	switch status {
	case JobStatusQueued:
		return "Queued"
	case JobStatusRunning:
		return "Running…"
	case JobStatusDone:
		return "Completed"
	case JobStatusFailed:
		return "Failed"
	default:
		return ""
	}
}
