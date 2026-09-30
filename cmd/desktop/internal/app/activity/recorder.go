package activity

import "context"

// Recorder reports audit events without propagating persistence failures.
type Recorder interface {
	Record(ctx context.Context, e Event)
}
