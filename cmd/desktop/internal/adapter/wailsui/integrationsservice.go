package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// IntegrationsService exposes the connector registry and each entry's
// connection state to the Integrations screen. The frontend re-reads it on
// connection:updated.
type IntegrationsService struct {
	integrations *app.IntegrationsService
}

func NewIntegrationsService(i *app.IntegrationsService) *IntegrationsService {
	return &IntegrationsService{integrations: i}
}

func (s *IntegrationsService) List(ctx context.Context) ([]app.Integration, error) {
	return s.integrations.List(ctx)
}
