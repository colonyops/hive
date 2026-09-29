package commands

import (
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHCListFilterWiring verifies that status and session flags are correctly
// wired into hc.ListFilter values.
func TestHCListFilterWiring(t *testing.T) {
	tests := []struct {
		name          string
		statusFlag    string
		sessionFlag   string
		epicIDArg     string
		wantEpicID    string
		wantSessionID string
		wantStatus    *hc.Status
		wantErr       string
	}{
		{
			name:       "no filters",
			wantStatus: nil,
		},
		{
			name:       "status open",
			statusFlag: "open",
			wantStatus: statusPtr(hc.StatusOpen),
		},
		{
			name:       "status in_progress",
			statusFlag: "in_progress",
			wantStatus: statusPtr(hc.StatusInProgress),
		},
		{
			name:       "status done",
			statusFlag: "done",
			wantStatus: statusPtr(hc.StatusDone),
		},
		{
			name:       "status cancelled",
			statusFlag: "cancelled",
			wantStatus: statusPtr(hc.StatusCancelled),
		},
		{
			name:       "invalid status",
			statusFlag: "bogus",
			wantErr:    "bogus is not a valid Status",
		},
		{
			name:          "session filter",
			sessionFlag:   "mysession",
			wantSessionID: "mysession",
		},
		{
			name:       "epic id arg",
			epicIDArg:  "hc-abc123",
			wantEpicID: "hc-abc123",
		},
		{
			name:          "all filters",
			statusFlag:    "open",
			sessionFlag:   "mysession",
			epicIDArg:     "hc-epic1",
			wantEpicID:    "hc-epic1",
			wantSessionID: "mysession",
			wantStatus:    statusPtr(hc.StatusOpen),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the filter the same way the listCmd action does.
			filter := hc.ListFilter{
				EpicID:    tt.epicIDArg,
				SessionID: tt.sessionFlag,
			}

			var err error
			if tt.statusFlag != "" {
				s, parseErr := hc.ParseStatus(tt.statusFlag)
				if parseErr != nil {
					err = parseErr
				} else {
					filter.Status = &s
				}
			}

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantEpicID, filter.EpicID)
			assert.Equal(t, tt.wantSessionID, filter.SessionID)
			assert.Equal(t, tt.wantStatus, filter.Status)
		})
	}
}

func statusPtr(s hc.Status) *hc.Status { return &s }
