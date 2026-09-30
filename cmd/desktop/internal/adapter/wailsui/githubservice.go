package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
)

// GitHubService exposes the GitHub connector's connection to the frontend:
// the device flow, the PAT fallback, and disconnecting. State changes are
// pushed via the connection:updated signal; the frontend re-reads Status on
// receipt.
//
// It is one connector's service, not the app's login. Nothing in the app is
// gated on it returning connected.
type GitHubService struct {
	github *app.GitHubService
}

func NewGitHubService(g *app.GitHubService) *GitHubService { return &GitHubService{github: g} }

func (s *GitHubService) Status() ghsource.ConnectionStatus {
	return s.github.Status(context.Background())
}

func (s *GitHubService) StartDeviceFlow(ctx context.Context) (ghsource.DeviceFlowInfo, error) {
	return s.github.StartDeviceFlow(ctx)
}

func (s *GitHubService) CancelDeviceFlow() { s.github.CancelDeviceFlow(context.Background()) }

func (s *GitHubService) SetToken(ctx context.Context, token string) (ghsource.ConnectionStatus, error) {
	return s.github.SetToken(ctx, token)
}

func (s *GitHubService) Disconnect() error { return s.github.Disconnect(context.Background()) }
