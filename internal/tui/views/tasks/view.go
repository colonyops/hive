package tasks

import (
	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/hive"
)

// View is the Bubble Tea sub-model for the tasks tab.
type View struct {
	svc    *hive.HoneycombService
	width  int
	height int
	active bool
}

// New creates a new tasks View.
func New(svc *hive.HoneycombService) *View {
	return &View{
		svc: svc,
	}
}

// Init initializes the tasks view.
func (v *View) Init() tea.Cmd {
	return nil
}

// Update handles messages for the tasks view.
func (v *View) Update(msg tea.Msg) tea.Cmd {
	_ = msg
	return nil
}

// View renders the tasks view.
func (v *View) View() string {
	if v.svc == nil {
		return "Tasks not configured"
	}
	return "No tasks loaded"
}

// SetSize updates the view dimensions.
func (v *View) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// SetActive sets whether this view is the currently active tab.
func (v *View) SetActive(active bool) {
	v.active = active
}
