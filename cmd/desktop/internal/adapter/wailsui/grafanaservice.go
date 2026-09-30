package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana"
)

// GrafanaService exposes the Grafana stack auth to the frontend. It is one
// connector's service, not the app's login: nothing in the app is gated on a
// stack being connected.
type GrafanaService struct {
	grafana *app.GrafanaService
}

func NewGrafanaService(g *app.GrafanaService) *GrafanaService { return &GrafanaService{grafana: g} }

func (s *GrafanaService) Connect(ctx context.Context, url, token string) (grafana.Stack, error) {
	return s.grafana.Connect(ctx, url, token)
}

func (s *GrafanaService) Disconnect(ctx context.Context, account string) error {
	return s.grafana.Disconnect(ctx, account)
}
