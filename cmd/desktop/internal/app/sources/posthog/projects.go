package posthog

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/bindingstore"
)

// ProjectStore persists each connected project's non-secret binding — host URL
// and numeric project id — keyed by credential ref (see bindingstore). The
// project id is bound alongside the host so a node cannot read a project the
// key was never connected to.
type ProjectStore = bindingstore.Store[Binding]

func NewProjectStore(path string) *ProjectStore {
	return bindingstore.New[Binding]("posthog projects", path)
}

// Binding is what a poll needs to address one connected project.
type Binding struct {
	URL       string `json:"url"`
	ProjectID int    `json:"projectID"`
	Name      string `json:"name,omitempty"`
}
