package jobs

import "context"

// Recorder tracks output-worker jobs without propagating persistence failures.
type Recorder interface {
	// Begin creates a queued job and returns its id, or zero when persistence
	// fails. Subsequent transitions reference this id.
	Begin(ctx context.Context, label, actionID, target string) int64
	// Running marks a job running and links its output_command id.
	Running(ctx context.Context, id int64, commandID int64)
	// Resume returns the running job linked to commandID, or zero when none can
	// be restored. It keeps automatic retries on one lifecycle across restarts.
	Resume(ctx context.Context, commandID int64) int64
	// Done marks a job completed.
	Done(ctx context.Context, id int64)
	// Fail marks a job failed with a reason.
	Fail(ctx context.Context, id int64, reason string)
}
