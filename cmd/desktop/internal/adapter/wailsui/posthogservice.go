package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog"
)

// PostHogService exposes the PostHog project auth to the frontend. It is one
// connector's service, not the app's login: nothing in the app is gated on a
// project being connected.
type PostHogService struct {
	posthog *app.PostHogService
}

func NewPostHogService(p *app.PostHogService) *PostHogService { return &PostHogService{posthog: p} }

// Projects validates the key and lists what it can reach without storing
// anything, so the connect dialog can offer a picker before the user commits.
func (s *PostHogService) Projects(ctx context.Context, url, token string) ([]posthog.Project, error) {
	return s.posthog.Projects(ctx, url, token)
}

func (s *PostHogService) Connect(ctx context.Context, url, token string, projectID int) (posthog.Project, error) {
	return s.posthog.Connect(ctx, url, token, projectID)
}

func (s *PostHogService) Disconnect(ctx context.Context, account string) error {
	return s.posthog.Disconnect(ctx, account)
}
