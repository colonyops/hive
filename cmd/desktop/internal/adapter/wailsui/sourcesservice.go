package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// SourcesService is the frontend's manual refresh. The feed's refresh command
// and its empty-state button call it before they re-read the list, so pressing
// refresh fetches instead of redrawing what was already stored.
type SourcesService struct {
	sources *app.SourcesService
}

func NewSourcesService(sources *app.SourcesService) *SourcesService {
	return &SourcesService{sources: sources}
}

// RefreshResult is what one manual tick was worth. Appended counts rows added
// to the event log, not inbox items the user will see: the engine routes them
// afterwards, and a flow may drop them.
type RefreshResult struct {
	Sources  int `json:"sources"`
	Appended int `json:"appended"`
	Failed   int `json:"failed"`
}

// Refresh drains every source now. It returns once the fetches are done, but
// the items they produced land asynchronously — the caller reloads on the
// "inbox:updated" event, not on this returning.
func (s *SourcesService) Refresh(ctx context.Context) (RefreshResult, error) {
	summary, err := s.sources.Refresh(ctx)
	if err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{Sources: summary.Sources, Appended: summary.Appended, Failed: summary.Failed}, nil
}
