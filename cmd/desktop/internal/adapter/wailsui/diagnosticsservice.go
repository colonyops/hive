package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

type DiagnosticsService struct {
	diagnostics *app.DiagnosticsService
	open        func()
}

func NewDiagnosticsService(diagnostics *app.DiagnosticsService, open func()) *DiagnosticsService {
	return &DiagnosticsService{diagnostics: diagnostics, open: open}
}

func (s *DiagnosticsService) Open() { s.open() }
func (s *DiagnosticsService) Read(ctx context.Context, q app.DiagnosticsQuery) (app.DiagnosticsSnapshot, error) {
	return s.diagnostics.Read(ctx, q)
}

func (s *DiagnosticsService) Context(ctx context.Context, req app.DiagnosticsIncident) (app.DiagnosticsContext, error) {
	return s.diagnostics.Context(ctx, req)
}

func (s *DiagnosticsService) Save(ctx context.Context, req app.DiagnosticsIncident) (app.DiagnosticsContext, error) {
	return s.diagnostics.Save(ctx, req)
}

func (s *DiagnosticsService) Prepare(ctx context.Context, req app.DiagnosticsIncident) (app.DiagnosticsContext, error) {
	return s.diagnostics.Prepare(ctx, req)
}

func (s *DiagnosticsService) Agents(ctx context.Context) app.DiagnosticsAgents {
	return s.diagnostics.Agents(ctx)
}

func (s *DiagnosticsService) Reveal(ctx context.Context, source string) error {
	return s.diagnostics.Reveal(ctx, source)
}
