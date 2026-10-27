package app

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/google/uuid"
)

func (s *DriveService) requestPane(ctx context.Context, request events.DriveToggleRequested) (string, error) {
	request.RequestID = uuid.NewString()
	result := make(chan string, 1)
	s.paneMu.Lock()
	s.panePending[request.RequestID] = result
	s.paneMu.Unlock()
	defer func() {
		s.paneMu.Lock()
		delete(s.panePending, request.RequestID)
		s.paneMu.Unlock()
	}()
	s.events.Publish(ctx, request)
	timer := time.NewTimer(s.paneTimeout)
	defer timer.Stop()
	select {
	case status := <-result:
		return status, nil
	case <-timer.C:
		return "no_ui_response", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *DriveService) AcknowledgePane(_ context.Context, requestID, status string) error {
	switch status {
	case "opened", "closed", "session_not_visible", "navigation_failed":
	default:
		return Errorf(KindInvalid, "invalid drive pane outcome %q", status)
	}
	s.paneMu.Lock()
	defer s.paneMu.Unlock()
	if result, ok := s.panePending[requestID]; ok {
		select {
		case result <- status:
		default:
		}
	}
	return nil
}
