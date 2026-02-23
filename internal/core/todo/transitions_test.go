package todo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    Status
		to      Status
		wantErr bool
	}{
		{name: "pending to acknowledged", from: StatusPending, to: StatusAcknowledged},
		{name: "pending to completed", from: StatusPending, to: StatusCompleted},
		{name: "pending to dismissed", from: StatusPending, to: StatusDismissed},
		{name: "acknowledged to completed", from: StatusAcknowledged, to: StatusCompleted},
		{name: "acknowledged to dismissed", from: StatusAcknowledged, to: StatusDismissed},
		{name: "completed to acknowledged", from: StatusCompleted, to: StatusAcknowledged, wantErr: true},
		{name: "dismissed to pending", from: StatusDismissed, to: StatusPending, wantErr: true},
		{name: "pending to pending", from: StatusPending, to: StatusPending, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTransition(tc.from, tc.to)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid transition")
				return
			}
			require.NoError(t, err)
		})
	}
}
