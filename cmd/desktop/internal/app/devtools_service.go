package app

// DevToolsService reports whether the developer-only diagnostic pane is
// reachable in this build (ADR developer-tools-are-reachable-in-a-shipped-build-behind-a-setting).
type DevToolsService struct {
	enabled bool
}

func newDevToolsService(enabled bool) *DevToolsService {
	return &DevToolsService{enabled: enabled}
}

// Enabled reports whether this build was asked to expose the developer tools.
func (s *DevToolsService) Enabled() bool { return s.enabled }
