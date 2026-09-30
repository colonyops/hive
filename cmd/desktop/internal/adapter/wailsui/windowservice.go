package wailsui

// WindowService exposes native window state to the frontend.
type WindowService struct {
	focus *FocusState
}

// NewWindowService constructs the service over the application's focus state.
func NewWindowService(focus *FocusState) *WindowService {
	return &WindowService{focus: focus}
}

// Focused reports whether the native application window is currently focused.
func (s *WindowService) Focused() bool {
	return s.focus.Get()
}
