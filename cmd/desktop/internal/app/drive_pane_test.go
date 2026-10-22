package app

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrivePaneOutcome(t *testing.T) {
	for _, outcome := range []string{"opened", "closed", "session_not_visible", "navigation_failed"} {
		t.Run(outcome, func(t *testing.T) {
			bus := newTestBus(t)
			svc := newDriveService(DriveDeps{Events: bus})
			cancel := events.Subscribe(t.Context(), bus, "test.ack", events.Buffer(8), func(ctx context.Context, e events.DriveToggleRequested) {
				assert.NotEmpty(t, e.RequestID)
				assert.NoError(t, svc.AcknowledgePane(ctx, "unrelated", "opened"))
				assert.NoError(t, svc.AcknowledgePane(ctx, e.RequestID, outcome))
			})
			t.Cleanup(cancel)
			got, err := svc.requestPane(t.Context(), events.DriveToggleRequested{HiveSession: "abc", Open: true})
			require.NoError(t, err)
			require.Equal(t, outcome, got)
			require.Empty(t, svc.panePending)
		})
	}
}

func TestDrivePaneTimeoutAndCancellation(t *testing.T) {
	svc := newDriveService(DriveDeps{Events: newTestBus(t)})
	svc.paneTimeout = time.Millisecond
	got, err := svc.requestPane(t.Context(), events.DriveToggleRequested{Open: true})
	require.NoError(t, err)
	require.Equal(t, "no_ui_response", got)
	require.Empty(t, svc.panePending)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = svc.requestPane(ctx, events.DriveToggleRequested{Open: true})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, svc.panePending)
	require.Error(t, svc.AcknowledgePane(t.Context(), "id", "invented"))
}
