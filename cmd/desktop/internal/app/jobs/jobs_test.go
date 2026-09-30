package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepFor(t *testing.T) {
	tests := []struct {
		status JobStatus
		want   string
	}{
		{JobStatusQueued, "Queued"},
		{JobStatusRunning, "Running…"},
		{JobStatusDone, "Completed"},
		{JobStatusFailed, "Failed"},
	}
	for _, test := range tests {
		t.Run(test.status.String(), func(t *testing.T) {
			assert.Equal(t, test.want, StepFor(test.status))
		})
	}
}
