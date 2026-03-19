package styles

import (
	"testing"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
)

func TestRenderStatusIndicator(t *testing.T) {
	tests := []struct {
		status    terminal.Status
		indicator string
	}{
		{terminal.StatusActive, StatusIndicatorActive},
		{terminal.StatusApproval, StatusIndicatorApproval},
		{terminal.StatusReady, StatusIndicatorReady},
		{terminal.StatusMissing, StatusIndicatorMissing},
		{terminal.Status("unknown"), StatusIndicatorMissing},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := RenderStatusIndicator(tt.status)
			assert.Contains(t, result, tt.indicator)
		})
	}
}

func TestRenderStatusIndicator_AllStatusesCovered(t *testing.T) {
	// Ensure all known statuses produce non-empty output
	statuses := []terminal.Status{
		terminal.StatusActive,
		terminal.StatusApproval,
		terminal.StatusReady,
		terminal.StatusMissing,
	}

	for _, s := range statuses {
		result := RenderStatusIndicator(s)
		assert.NotEmpty(t, result, "status %q should produce non-empty output", s)
	}
}
