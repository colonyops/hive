package wailsui

import "github.com/colonyops/hive/cmd/desktop/internal/app"

type DiagnosticsService struct {
	*app.DiagnosticsService
	open func()
}

func NewDiagnosticsService(diagnostics *app.DiagnosticsService, open func()) *DiagnosticsService {
	return &DiagnosticsService{DiagnosticsService: diagnostics, open: open}
}

func (s *DiagnosticsService) Open() { s.open() }
