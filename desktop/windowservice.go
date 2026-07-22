package main

// WindowService exposes native window state to the frontend.
type WindowService struct {
	focus *focusState
}

// NewWindowService constructs the service over the application's focus state.
func NewWindowService(focus *focusState) *WindowService {
	return &WindowService{focus: focus}
}

// Focused reports whether the native application window is currently focused.
func (s *WindowService) Focused() bool {
	return s.focus.get()
}
