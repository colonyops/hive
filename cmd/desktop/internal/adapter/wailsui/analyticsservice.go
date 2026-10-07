package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

type AnalyticsService struct{ analytics *app.AnalyticsService }

func NewAnalyticsService(analytics *app.AnalyticsService) *AnalyticsService {
	return &AnalyticsService{analytics: analytics}
}

func (s *AnalyticsService) Summary(ctx context.Context) (app.AnalyticsSummary, error) {
	return s.analytics.Summary(ctx)
}

func (s *AnalyticsService) SetEnabled(ctx context.Context, enabled bool) error {
	return s.analytics.SetEnabled(ctx, enabled)
}
func (s *AnalyticsService) Clear(ctx context.Context) error { return s.analytics.Clear(ctx) }
