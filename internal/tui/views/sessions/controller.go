package sessions

import (
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
)

// Controller manages session data and filtering.
// It contains pure data logic with no Bubble Tea dependencies.
type Controller struct {
	allSessions  []session.Session
	statusFilter terminal.Status
	localRemote  string
}

// NewController creates a new sessions controller.
func NewController() *Controller {
	return &Controller{}
}

// SetSessions replaces the current sessions list.
func (c *Controller) SetSessions(sessions []session.Session) {
	c.allSessions = sessions
}

// AllSessions returns all sessions.
func (c *Controller) AllSessions() []session.Session {
	return c.allSessions
}

// FindByID returns the session with the given ID, or nil if not found.
func (c *Controller) FindByID(id string) *session.Session {
	for i := range c.allSessions {
		if c.allSessions[i].ID == id {
			return &c.allSessions[i]
		}
	}
	return nil
}

// SetStatusFilter sets the terminal status filter.
func (c *Controller) SetStatusFilter(status terminal.Status) {
	c.statusFilter = status
}

// StatusFilter returns the current terminal status filter.
func (c *Controller) StatusFilter() terminal.Status {
	return c.statusFilter
}

// LocalRemote returns the local remote URL used for highlighting.
func (c *Controller) LocalRemote() string {
	return c.localRemote
}

// SetLocalRemote sets the local remote URL.
func (c *Controller) SetLocalRemote(remote string) {
	c.localRemote = remote
}
