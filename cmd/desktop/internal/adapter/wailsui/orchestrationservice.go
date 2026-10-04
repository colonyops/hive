package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

type OrchestrationService struct {
	orchestration *app.OrchestrationService
}

func NewOrchestrationService(o *app.OrchestrationService) *OrchestrationService {
	return &OrchestrationService{orchestration: o}
}

func (s *OrchestrationService) ListTokens(ctx context.Context) ([]stores.OrchestratorToken, error) {
	return s.orchestration.Tokens(ctx)
}

func (s *OrchestrationService) CreateToken(ctx context.Context, name string) (app.CreatedOrchestratorToken, error) {
	return s.orchestration.CreateToken(ctx, name)
}

func (s *OrchestrationService) RevokeToken(ctx context.Context, id int64) error {
	return s.orchestration.RevokeToken(ctx, id)
}

func (s *OrchestrationService) ServerURL(ctx context.Context) string {
	return s.orchestration.ServerURL(ctx)
}
