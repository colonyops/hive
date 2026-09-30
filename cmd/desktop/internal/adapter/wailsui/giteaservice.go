package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea"
)

// GiteaService exposes the Gitea instance auth to the frontend. It is one
// connector's service, not the app's login: nothing in the app is gated on an
// instance being connected.
type GiteaService struct {
	gitea *app.GiteaService
}

func NewGiteaService(g *app.GiteaService) *GiteaService { return &GiteaService{gitea: g} }

// Connect validates the instance URL and token, then stores the credential. It
// returns the account it resolved, which is the ref half a source node names.
func (s *GiteaService) Connect(ctx context.Context, url, token string) (gitea.Instance, error) {
	return s.gitea.Connect(ctx, url, token)
}

func (s *GiteaService) Disconnect(ctx context.Context, account string) error {
	return s.gitea.Disconnect(ctx, account)
}
